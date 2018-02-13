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

const (
	membersBucketName = "members"
	guidIdxBucketName = "guids"
	dnIdxBucketName   = "dns"
)

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
	rootBucketName []byte
	db             *bolt.DB
}

// New ...
func New(db *bolt.DB) Users {
	return Users{
		rootBucketName: []byte("ldap-scim"),
		db:             db,
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
	root, err := tx.CreateBucketIfNotExists([]byte(u.rootBucketName))
	if err != nil {
		return fmt.Errorf("create %s bucket: %s", u.rootBucketName, err)
	}

	// create membership bucket
	_, err = root.CreateBucketIfNotExists([]byte(membersBucketName))
	if err != nil {
		return fmt.Errorf("create members bucket: %s", err)
	}

	// create DN-to-GUID index
	_, err = root.CreateBucketIfNotExists([]byte(guidIdxBucketName))
	if err != nil {
		return fmt.Errorf("create guids index bucket: %s", err)
	}

	// create GUID-to-DN index
	_, err = root.CreateBucketIfNotExists([]byte(dnIdxBucketName))
	if err != nil {
		return fmt.Errorf("create dns bucket: %s", err)
	}

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

	root := tx.Bucket(u.rootBucketName)
	dnIdx := root.Bucket([]byte(dnIdxBucketName))

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

	root := tx.Bucket(u.rootBucketName)
	guidIdx := root.Bucket([]byte(guidIdxBucketName))

	dn := guidIdx.Get([]byte(guid))
	// if len(dn) == 0 {
	// 	return "", fmt.Errorf("GetDN(%s) failed: not found", guid)
	// }
	return string(dn), nil
}

// GetMemberDNs ...
func (u *Users) GetMemberDNs() ([]string, error) {
	dns := []string{}

	tx, err := u.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	root := tx.Bucket(u.rootBucketName)
	guidIdx := root.Bucket([]byte(guidIdxBucketName))

	if err := guidIdx.ForEach(func(k []byte, v []byte) error {
		dns = append(dns, string(k))
		return nil
	}); err != nil {
		return nil, err
	}

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
	root := tx.Bucket(u.rootBucketName)
	members := root.Bucket([]byte(membersBucketName))
	guidIdx := root.Bucket([]byte(guidIdxBucketName))
	dnIdx := root.Bucket([]byte(dnIdxBucketName))

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
	// Start the transaction.
	tx, err := u.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Retrieve the root->members bucket.
	root := tx.Bucket(u.rootBucketName)
	members := root.Bucket([]byte(membersBucketName))
	guidIdx := root.Bucket([]byte(guidIdxBucketName))
	dnIdx := root.Bucket([]byte(dnIdxBucketName))

	// remove membership
	members.Delete([]byte(user.GUID))

	// clear indexes
	dnIdx.Delete([]byte(user.DN))
	guidIdx.Delete([]byte(user.GUID))

	// Commit the transaction.
	if err := tx.Commit(); err != nil {
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

	root := tx.Bucket([]byte(u.rootBucketName))
	members := root.Bucket([]byte(membersBucketName))
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
