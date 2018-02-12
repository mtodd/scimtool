package users

import (
	"encoding/json"
	"fmt"

	"github.com/boltdb/bolt"
	scim "github.com/mtodd/scimtool"
)

/*

# Database structure

## Members

* state(?)
* guid (key)
* userName
* firstName
* lastName
* email

## Indexes

* dn-to-guid
* guid-to-dn

*/

// User ...
type User struct {
	DN        string
	GUID      string
	UserName  string
	FirstName string
	LastName  string
	Email     string
}

// Users ...
type Users struct {
	store   string
	db      *bolt.DB
	root    *bolt.Bucket
	members *bolt.Bucket
	dnIdx   *bolt.Bucket
	guidIdx *bolt.Bucket
}

// New ...
func New(db *bolt.DB) Users {
	return Users{
		store: "ldap-scim",
		db:    db,
	}
}

// Prepare ...
func (u *Users) Prepare() error {
	// Start the transaction.
	tx, err := u.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// create the root IdP bucket.
	root, err := tx.CreateBucketIfNotExists([]byte(u.store))
	if err != nil {
		return fmt.Errorf("create ldap-scim bucket: %s", err)
	}

	// create membership bucket
	mb, err := root.CreateBucketIfNotExists([]byte("members"))
	if err != nil {
		return fmt.Errorf("create members bucket: %s", err)
	}

	// create DN-to-GUID index
	guids, err := root.CreateBucketIfNotExists([]byte("guids"))
	if err != nil {
		return fmt.Errorf("create guids index bucket: %s", err)
	}

	// create GUID-to-DN index
	dns, err := root.CreateBucketIfNotExists([]byte("dns"))
	if err != nil {
		return fmt.Errorf("create dns bucket: %s", err)
	}

	u.root = root
	u.members = mb
	u.guidIdx = guids
	u.dnIdx = dns

	// Commit the transaction.
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// GetGUID ...
func (u *Users) GetGUID(dn string) (string, error) {
	tx, err := u.db.Begin(false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	root := tx.Bucket([]byte("ldap-scim"))
	dnIdx := root.Bucket([]byte("dns"))

	guid := dnIdx.Get([]byte(dn))
	// if len(guid) == 0 {
	// 	return "", fmt.Errorf("GetGUID(%s) failed: not found", dn)
	// }
	return string(guid), nil
}

// GetDN ...
func (u *Users) GetDN(guid string) (string, error) {
	tx, err := u.db.Begin(false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	root := tx.Bucket([]byte("ldap-scim"))
	guidIdx := root.Bucket([]byte("guids"))

	dn := guidIdx.Get([]byte(guid))
	// if len(dn) == 0 {
	// 	return "", fmt.Errorf("GetDN(%s) failed: not found", guid)
	// }
	return string(dn), nil
}

// GetMemberDNs ...
func (u *Users) GetMemberDNs() ([]string, error) {
	dns := []string{}
	u.guidIdx.ForEach(func(k []byte, v []byte) error {
		dns = append(dns, string(k))
		return nil
	})
	return nil, nil
}

// Add ...
func (u *Users) Add(dn string, user scim.User) error {
	// Start the transaction.
	tx, err := u.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dnb := []byte(dn)
	guid := []byte(user.ID)

	// Retrieve the root->members bucket.
	root := tx.Bucket([]byte("ldap-scim"))
	members := root.Bucket([]byte("members"))
	guidIdx := root.Bucket([]byte("guids"))
	dnIdx := root.Bucket([]byte("dns"))

	// Marshal and save the encoded user.
	if buf, err := json.Marshal(user); err != nil {
		return err
	} else if err := members.Put(guid, buf); err != nil {
		return err
	}

	// write GUID-to-DN index
	if err := guidIdx.Put(guid, dnb); err != nil {
		return fmt.Errorf("index guid(%s, %s): %s", guid, dn, err)
	}

	// write DN-to-GUID index
	if err := dnIdx.Put(dnb, guid); err != nil {
		return fmt.Errorf("index dn(%s, %s): %s", dn, guid, err)
	}

	// Commit the transaction.
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// Delete ...
func (u *Users) Delete(user User) error {
	if err := u.db.Update(func(tx *bolt.Tx) error {
		// remove membership
		u.members.Delete([]byte(user.GUID))

		// clear indexes
		u.dnIdx.Delete([]byte(user.DN))
		u.guidIdx.Delete([]byte(user.GUID))

		return nil
	}); err != nil {
		return err
	}

	return nil
}

// List ...
func (u *Users) List() ([]scim.User, error) {
	list := make([]scim.User, 0)

	tx, err := u.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	root := tx.Bucket([]byte(u.store))
	members := root.Bucket([]byte("members"))
	if err := members.ForEach(func(k []byte, v []byte) error {
		u := scim.User{}
		// log.Printf("%+v %+v", string(k), string(v))
		if err := json.Unmarshal(v, &u); err != nil {
			return err
		}

		list = append(list, u)

		return nil
	}); err != nil {
		return nil, err
	}

	return list, nil
}
