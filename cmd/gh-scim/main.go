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
gh-scim <command> -o <org> [guid]

commands:
* list [filter]
  [filter] is a SCIM filter
  example: 'userName eq "evilmtodd"'
* remove [guid]
  [guid] is required
* add...

flags:
* -o <org>: the organization name, e.g. "acme"; required for all commands
* -d: debug logging
`

const defaultAPIURL = "https://api.github.com"

type apiClient struct {
	client  *http.Client
	baseURL string
	token   string
	org     string
	debug   bool
}

func (c *apiClient) buildRequest(method, endpoint string) (*http.Request, error) {
	req, err := http.NewRequest(method, c.buildEndpointURL(endpoint), nil)
	// http.NewRequest

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
	// log.Print(req)
	return c.client.Do(req)
}

// {"schemas":["urn:ietf:params:scim:api:messages:2.0:ListResponse"],"totalResults":0,"itemsPerPage":0,"startIndex":1,"Resources":[]}
// { "schemas":["urn:ietf:params:scim:api:messages:2.0:ListResponse"],
//   "totalResults":2,
//   "itemsPerPage":2,
//   "startIndex":1,
//   "Resources":[
//     {
//       "schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
//       "id":"e7818cf4-0206-11e8-8526-afbcdd6f73fd",
//       "externalId":"evilmtodd",
//       "userName":"evilmtodd",
//       "name":{"givenName":"Mtodd","familyName":"Evil"},
//       "emails":[{"value":"chiology+evilmtodd@gmail.com","type":"work","primary":true}],
//       "active":true,
//       "meta":{
// "resourceType":"User","created":"2018-01-25T14:35:31-05:00","lastModified":"2018-01-25T14:35:31-05:00","location":"https://api.github.com/scim/v2/organizations/GH4B/Users/e7818cf4-0206-11e8-8526-afbcdd6f73fd"}},
//     {"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"id":"d9ff5ddc-0200-11e8-88b5-b07393a906c1","externalId":"00udrnla8jKyYID8F0h7","userName":"mtodd@github.com","name":{"givenName":"Matt","familyName":"Todd"},"emails":[{"value":"mtodd@github.com","primary":true,"type":"work"}],"active":true,"meta":{"resourceType":"User","created":"2018-01-25T13:52:11-05:00","lastModified":"2018-01-25T13:52:25-05:00","location":"https://api.github.com/scim/v2/organizations/GH4B/Users/d9ff5ddc-0200-11e8-88b5-b07393a906c1"}}
//   ]
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
//   "meta":{"resourceType":"User","created":"2018-01-25T14:35:31-05:00","lastModified":"2018-01-25T14:35:31-05:00","location":"https://api.github.com/scim/v2/organizations/GH4B/Users/e7818cf4-0206-11e8-8526-afbcdd6f73fd"}
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
func listHandler(client *apiClient, filter string) error {
	req, err := client.buildRequest("GET", fmt.Sprintf("/scim/v2/organizations/%s/Users", client.org))
	if err != nil {
		return err
	}

	if len(filter) > 0 {
		q := req.URL.Query()
		q.Add("filter", url.QueryEscape(filter))
		req.URL.RawQuery = q.Encode()
	}

	res, err := client.do(req)
	if err != nil {
		return err
	}

	// log.Println(res)

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

	// log.Printf("%v", body)
	// log.Println(string(body))

	var list scimListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return err
	}

	// log.Printf("%v", list)

	// var f interface{}
	// if err := json.Unmarshal(body, &f); err != nil {
	// 	return err
	// }
	// log.Printf("%v", f)

	for _, user := range list.Resources {
		json, err := json.Marshal(user)
		if err != nil {
			return err
		}
		// log.Printf("%s", json)
		fmt.Println(string(json))
	}

	return nil
}

// DELETE /scim/v2/organizations/:organization/Users/:id
func removeHandler(client *apiClient, guid string) error {
	req, err := client.buildRequest("DELETE", fmt.Sprintf("/scim/v2/organizations/%s/Users/%s", client.org, guid))
	if err != nil {
		return err
	}

	res, err := client.do(req)
	if err != nil {
		return err
	}

	// log.Println(res) // debugging

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

func addHandler(client *apiClient) error {
	req, err := client.buildRequest("POST", fmt.Sprintf("/scim/v2/organizations/%s/Users", client.org))
	if err != nil {
		return err
	}

	req.Body = ioutil.NopCloser(bytes.NewBufferString(addScimUserTmpl))

	// log.Printf("%v", req)

	res, err := client.do(req)
	if err != nil {
		return err
	}

	// log.Printf("%v", res)

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("remove failed: %v", res)
	}

	// log.Printf("%v", body)
	// log.Println(string(body))

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
		baseURL: defaultAPIURL,
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

		err = listHandler(client, filter)
	case "remove":
		if flag.Arg(1) == "" {
			log.Fatalf("error: guid is required\n\n%s", usage)
		}

		guid := flag.Arg(1)
		err = removeHandler(client, guid)
	case "add":
		err = addHandler(client)
	default:
		err = fmt.Errorf("unknown command")
	}

	if err != nil {
		log.Fatalf("error: %s\n\n%s", err, usage)
	}
}
