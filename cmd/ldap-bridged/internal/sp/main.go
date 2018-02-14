package sp

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	scim "github.com/mtodd/scimtool"
)

const defaultBaseURL = "https://api.github.com"

type fakeAPIClient struct {
	store map[string]scim.User
}

func (c *fakeAPIClient) Add(u scim.User) (string, error) {
	h := sha256.New()
	h.Write([]byte(u.UserName))
	guid := base64.StdEncoding.EncodeToString(h.Sum(nil))

	log.Printf("scim: adding %s as %s", u.UserName, guid)

	u.ID = guid
	c.store[guid] = u

	return guid, nil
}

func (c *fakeAPIClient) Del(guid string) error {
	log.Printf("scim: removing %s", guid)

	delete(c.store, guid)

	return nil
}

func (c *fakeAPIClient) List() ([]scim.User, error) {
	list := make([]scim.User, len(c.store))

	for _, user := range c.store {
		list = append(list, user)
	}

	return list, nil
}

type apiClient struct {
	client  *http.Client
	baseURL string
	token   string
	org     string
	debug   bool
}

func (c *apiClient) buildRequest(method, endpoint string) (*http.Request, error) {
	req, err := http.NewRequest(method, c.buildEndpointURL(endpoint), nil)

	req.Header.Set("Accept", "application/vnd.github.cloud-9-preview+json+scim")
	req.Header.Set("Authorization", "Bearer "+c.token)

	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, err
}

func (c *apiClient) buildEndpointURL(path string) string {
	return fmt.Sprintf("%s%s", c.baseURL, path)
}

func (c *apiClient) do(req *http.Request) (*http.Response, error) {
	if c.debug {
		log.Printf("debug: %v", req)
	}

	res, err := c.client.Do(req)

	if c.debug && err == nil {
		log.Printf("debug: %v", res)
	}

	return res, err
}

func (c *apiClient) Add(user scim.User) (string, error) {
	req, err := c.buildRequest("POST", fmt.Sprintf("/scim/v2/organizations/%s/Users", c.org))
	if err != nil {
		return "", err
	}

	jsonBody, err := json.Marshal(user)
	if err != nil {
		return "", err
	}

	req.Body = ioutil.NopCloser(bytes.NewBufferString(string(jsonBody)))

	res, err := c.do(req)
	if err != nil {
		return "", err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("remove failed: %v", res)
	}

	if c.debug {
		log.Printf("debug: %v", string(body))
	}

	if err := json.Unmarshal(body, &user); err != nil {
		return "", err
	}

	log.Printf("added: %s", user.ID)

	return user.ID, nil
}

func (c *apiClient) Del(guid string) error {
	req, err := c.buildRequest("DELETE", fmt.Sprintf("/scim/v2/organizations/%s/Users/%s", c.org, guid))
	if err != nil {
		return err
	}

	res, err := c.do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("remove failed: %v", res)
	}

	log.Printf("removed %s", guid)
	return nil
}

func (c *apiClient) List() ([]scim.User, error) {
	req, err := c.buildRequest("GET", fmt.Sprintf("/scim/v2/organizations/%s/Users", c.org))
	if err != nil {
		return nil, err
	}

	// include filter query param if filter is given
	// if len(filter) > 0 {
	// 	q := req.URL.Query()
	// 	q.Add("filter", filter)
	// 	req.URL.RawQuery = q.Encode()
	// }

	res, err := c.do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("list: bad request: %s", string(body))
	}

	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("list: not found: %s", string(body))
	}

	if c.debug {
		log.Printf("debug: %v", string(body))
	}

	var list scim.ListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}

	return list.Resources, nil
}

type scimProvider interface {
	Add(scim.User) (string, error)
	Del(guid string) error
	List() ([]scim.User, error)
}

// SCIMProvider ...
type SCIMProvider struct {
	client *scimProvider
	cfg    scimProviderConfig
}

type scimProviderConfig struct {
	token   string
	baseURL string
	org     string
	dryRun  bool
}

// NewSCIMProvider ...
func NewSCIMProvider(org, token string, dryRun bool) SCIMProvider {
	baseURL := os.Getenv("SCIM_BASEURL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	var client scimProvider

	if dryRun {
		client = &fakeAPIClient{
			store: make(map[string]scim.User),
		}
	} else {
		// HTTP client
		client = &apiClient{
			client:  &http.Client{},
			baseURL: baseURL,
			token:   token,
			org:     org,
		}
	}

	return SCIMProvider{
		client: &client,
	}
}

// Add ...
func (sp *SCIMProvider) Add(u scim.User) (string, error) {
	client := *sp.client
	guid, err := client.Add(u)
	if err != nil {
		return "", err
	}

	return guid, nil
}

// Del ...
func (sp *SCIMProvider) Del(guid string) error {
	client := *sp.client
	if err := client.Del(guid); err != nil {
		return err
	}

	return nil
}

// List ...
func (sp *SCIMProvider) List() ([]scim.User, error) {
	client := *sp.client
	list, err := client.List()
	if err != nil {
		return nil, err
	}

	return list, nil
}
