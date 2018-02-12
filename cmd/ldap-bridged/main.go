package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/boltdb/bolt"

	scim "github.com/mtodd/scimtool"
	"github.com/mtodd/scimtool/cmd/ldap-bridged/internal/db"
	"github.com/mtodd/scimtool/cmd/ldap-bridged/internal/idp"
	"github.com/mtodd/scimtool/cmd/ldap-bridged/internal/sp"

	ldap "gopkg.in/ldap.v2"
)

type bridge struct {
	idp   idp.LDAPProvider
	sp    sp.SCIMProvider
	db    *bolt.DB
	users users.Users
}

func newBridge(idp idp.LDAPProvider, sp sp.SCIMProvider, db *bolt.DB) bridge {
	return bridge{
		idp: idp,
		sp:  sp,
		db:  db,
	}
}

func (b *bridge) Open() error {
	b.users = users.New(b.db)
	b.users.Prepare()

	return nil
}

func (b *bridge) Start() {
	go b.run()
	b.idp.Start()
}

func (b *bridge) run() {
	for {
		select {
		case dn := <-b.idp.Added:
			b.Add(dn)
		case dn := <-b.idp.Removed:
			b.Del(dn)
		}
	}
}

func (b *bridge) Add(dn string) {
	log.Printf("add: %s", dn)

	// fetch LDAP User
	entry, err := b.idp.Fetch(dn)
	if err != nil {
		return
	}
	entry.PrettyPrint(2)
	// log.Printf("%+v", entry)

	// build SCIM User representation (map LDAP to SCIM attributes)
	user, _ := b.mapEntry(entry)
	log.Printf("%+v", user)

	// write to SCIM
	guid, err := b.sp.Add(user)
	if err != nil {
		log.Printf("add: scim failed: %s", err)
	}

	// receive GUID
	user.ID = guid

	// persist membership
	// persist DN-to-GUID mapping
	// persist GUID-to-DN mapping
	if err = b.users.Add(dn, user); err != nil {
		log.Printf("add: bridge store failed: %s", err)
	}
}

func (b *bridge) Del(dn string) {
	log.Printf("remove: %s", dn)
}

// mapEntry takes an LDAP entry, maps to a SCIM user representation
func (b *bridge) mapEntry(entry *ldap.Entry) (scim.User, error) {
	// u := users.User{
	// 	DN:        entry.DN,
	// 	GUID:      "",
	// 	UserName:  entry.GetAttributeValue("uid"),
	// 	FirstName: entry.GetAttributeValue("givenName"),
	// 	LastName:  entry.GetAttributeValue("sn"),
	// 	Email:     entry.GetAttributeValue("mail"),
	// }

	user := scim.User{
		Schemas:  []string{scim.UserSchema},
		UserName: entry.GetAttributeValue("uid"),
		Name: scim.Name{
			GivenName:  entry.GetAttributeValue("givenName"),
			FamilyName: entry.GetAttributeValue("sn"),
		},
		Emails: []scim.Email{{
			Type:    "work",
			Value:   entry.GetAttributeValue("mail"),
			Primary: true,
		}},
		Active: true,
	}

	return user, nil
}

func main() {
	conn, err := ldap.Dial("tcp", "localhost:389")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err = conn.Bind("cn=admin,dc=planetexpress,dc=com", "GoodNewsEveryone"); err != nil {
		log.Fatal(err)
	}

	db, err := bolt.Open("bridge.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	lb := idp.NewLDAPProvider(conn)
	sp := sp.NewSCIMProvider()
	b := newBridge(lb, sp, db)

	if err = b.Open(); err != nil {
		log.Fatal(err)
	}

	// run until SIGINT is triggered
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt)

	b.Start()

	<-term
}
