package scim

// ListResponse maps to the "ListResponse"
// (urn:ietf:params:scim:api:messages:2.0:ListResponse) SCIM type.
//
// { "schemas":["urn:ietf:params:scim:api:messages:2.0:ListResponse"],
//   "totalResults":2,
//   "itemsPerPage":2,
//   "startIndex":1,
//   "Resources":[...]
// }
type ListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	ItemsPerPage int      `json:"itemsPerPage"`
	StartIndex   int      `json:"startIndex"`
	Resources    []User
}

// User maps to the "User" (urn:ietf:params:scim:schemas:core:2.0:User) SCIM type.
//
// {
//   "schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],
//   "id":"e7818cf4-0206-11e8-8526-afbcdd6f73fd",
//   "externalId":"evilmtodd",
//   "userName":"evilmtodd",
//   "name":{"givenName":"Mtodd","familyName":"Evil"},
//   "emails":[{"value":"chiology+evilmtodd@gmail.com","type":"work","primary":true}],
//   "active":true,
//   "meta":{...}
// }
type User struct {
	Schemas    []string      `json:"schemas"`
	ID         string        `json:"id"`
	ExternalID string        `json:"externalId"`
	UserName   string        `json:"userName"`
	Name       interface{}   `json:"name"`
	Emails     []interface{} `json:"emails"`
	Active     bool          `json:"active"`
	Metadata   Metadata      `json:"meta"`
}

// Metadata maps to "meta" data.
//
// {
//   "resourceType":"User",
//   "created":"2018-01-25T14:35:31-05:00",
//   "lastModified":"2018-01-25T14:35:31-05:00",
//   "location":"https://api.github.com/scim/v2/organizations/GH4B/Users/e7818cf4-0206-11e8-8526-afbcdd6f73fd"
// }
type Metadata struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created"`
	LastModified string `json:"lastModified"`
	Location     string `json:"location"`
}
