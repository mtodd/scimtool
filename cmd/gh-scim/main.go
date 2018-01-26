package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
)

const usage = `
gh-scim <command> -o <org> [guid|filter]

commands:
* list [filter]
  [filter] is a SCIM filter
  example: 'userName eq "evilmtodd"'
* remove [guid]
  [guid] is required
* add...

environment variables:
* TOKEN: used to authenticate requests; required
* BASEURL: the API base URL; defaults to "https://api.github.com/"

flags:
* -o <org>: the organization name, e.g. "acme"; required for all commands
* -d: debug logging
`

const defaultBaseURL = "https://api.github.com"

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

// { "schemas":["urn:ietf:params:scim:api:messages:2.0:ListResponse"],
//   "totalResults":2,
//   "itemsPerPage":2,
//   "startIndex":1,
//   "Resources":[...]
// }
type scimListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	ItemsPerPage int      `json:"itemsPerPage"`
	StartIndex   int      `json:"startIndex"`
	Resources    []scimResource
}

// {
//   "schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
//   "id":"e7818cf4-0206-11e8-8526-afbcdd6f73fd",
//   "externalId":"evilmtodd",
//   "userName":"evilmtodd",
//   "name":{"givenName":"Mtodd","familyName":"Evil"},
//   "emails":[{"value":"chiology+evilmtodd@gmail.com","type":"work","primary":true}],
//   "active":true,
//   "meta":{
//     "resourceType":"User",
//     "created":"2018-01-25T14:35:31-05:00",
//     "lastModified":"2018-01-25T14:35:31-05:00",
//     "location":"https://api.github.com/scim/v2/organizations/GH4B/Users/e7818cf4-0206-11e8-8526-afbcdd6f73fd"
//   }
// }
type scimResource struct {
	Schemas    []string      `json:"schemas"`
	ID         string        `json:"id"`
	ExternalID string        `json:"externalId"`
	UserName   string        `json:"userName"`
	Name       interface{}   `json:"name"`
	Emails     []interface{} `json:"emails"`
	Active     bool          `json:"active"`
}

// GET https://api.github.com/scim/v2/organizations/:organization/Users
func (c *apiClient) listHandler(filter string) error {
	req, err := c.buildRequest("GET", fmt.Sprintf("/scim/v2/organizations/%s/Users", c.org))
	if err != nil {
		return err
	}

	if len(filter) > 0 {
		q := req.URL.Query()
		q.Add("filter", url.QueryEscape(filter))
		req.URL.RawQuery = q.Encode()
	}

	res, err := c.do(req)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("list: bad request: %s", string(body))
	}

	if res.StatusCode == http.StatusNotFound {
		return fmt.Errorf("list: not found: %s", string(body))
	}

	if c.debug {
		log.Printf("debug: %v", string(body))
	}

	var list scimListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return err
	}

	for _, user := range list.Resources {
		json, err := json.Marshal(user)
		if err != nil {
			return err
		}

		fmt.Println(string(json))
	}

	return nil
}

// DELETE /scim/v2/organizations/:organization/Users/:id
func (c *apiClient) removeHandler(guid string) error {
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

const addScimUserTmpl = `
{
  "schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
  "externalId":"evilmtodd",
  "userName":"evilmtodd",
  "name":{
    "familyName":"Evil",
    "givenName":"Mtodd"
  },
  "emails":[
    {
      "value":"chiology+evilmtodd@gmail.com",
      "type":"work",
      "primary":true
    }
  ]
}
`

// {
//   "schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
//   "id":"e7818cf4-0206-11e8-8526-afbcdd6f73fd",
//   "externalId":"evilmtodd",
//   "userName":"evilmtodd",
//   "name":{"givenName":"Mtodd","familyName":"Evil"},
//   "emails":[{"value":"chiology+evilmtodd@gmail.com","type":"work","primary":true}],
//   "active":true,
//   "meta":{"resourceType":"User","created":"2018-01-25T14:35:31-05:00","lastModified":"2018-01-25T14:35:31-05:00","location":"https://api.github.com/scim/v2/organizations/GH4B/Users/e7818cf4-0206-11e8-8526-afbcdd6f73fd"}
// }
type scimUser struct {
	scimResource
}

func (c *apiClient) addHandler() error {
	req, err := c.buildRequest("POST", fmt.Sprintf("/scim/v2/organizations/%s/Users", c.org))
	if err != nil {
		return err
	}

	req.Body = ioutil.NopCloser(bytes.NewBufferString(addScimUserTmpl))

	res, err := c.do(req)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("remove failed: %v", res)
	}

	if c.debug {
		log.Printf("debug: %v", string(body))
	}

	var user scimUser
	if err := json.Unmarshal(body, &user); err != nil {
		return err
	}

	log.Printf("added: %s", user.ID)

	return nil
}

func main() {
	var err error

	// configuration
	token := os.Getenv("TOKEN")

	baseURL := os.Getenv("BASEURL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	org := flag.String("o", "", "")
	debug := flag.Bool("d", false, "")
	flag.Parse()

	if *org == "" {
		log.Fatalf("error: -o organization flag is required\n\n%s", usage)
	}

	if token == "" {
		log.Fatalf("error: TOKEN environment variable is required\n\n%s", usage)
	}

	if len(flag.Args()) < 1 {
		log.Fatalf("error: command required\n\n%s", usage)
	}

	// HTTP client
	client := &apiClient{
		client:  &http.Client{},
		baseURL: baseURL,
		token:   token,
		org:     *org,
		debug:   *debug,
	}

	switch flag.Arg(0) {
	case "list":
		var filter string
		if flag.Arg(1) != "" {
			filter = flag.Arg(1)
		}

		err = client.listHandler(filter)
	case "remove":
		if flag.Arg(1) == "" {
			log.Fatalf("error: guid is required\n\n%s", usage)
		}

		guid := flag.Arg(1)
		err = client.removeHandler(guid)
	case "add":
		err = client.addHandler()
	default:
		log.Fatalf("error: unknown command\n\n%s", usage)
	}

	if err != nil {
		log.Fatalf("error: %s", err)
	}
}
