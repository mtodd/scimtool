package sp

import (
	"crypto/sha256"
	"encoding/base64"
	"log"

	scim "github.com/mtodd/scimtool"
)

// scim "github.com/imulab/go-scim/shared"

// SCIMProvider ...
type SCIMProvider struct {
	store map[string]scim.User
}

// NewSCIMProvider ...
func NewSCIMProvider() SCIMProvider {
	return SCIMProvider{
		store: make(map[string]scim.User),
	}
}

// Add ...
func (sp *SCIMProvider) Add(u scim.User) (string, error) {
	h := sha256.New()
	h.Write([]byte(u.UserName))
	guid := base64.StdEncoding.EncodeToString(h.Sum(nil))

	log.Printf("scim: adding %s as %s", u.UserName, guid)

	u.ID = guid
	sp.store[guid] = u

	return guid, nil
}

// Del ...
func (sp *SCIMProvider) Del(guid string) error {
	log.Printf("scim: removing %s", guid)

	delete(sp.store, guid)

	return nil
}

// List ...
func (sp *SCIMProvider) List() ([]scim.User, error) {
	list := make([]scim.User, len(sp.store))

	for _, user := range sp.store {
		list = append(list, user)
	}

	return list, nil
}
