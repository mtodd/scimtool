package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
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

func (b *bridge) Init() error {
	b.users = users.New(b.db)
	if err := b.users.Prepare(); err != nil {
		return err
	}

	return nil
}

// Sync ensures the bridge and SP are up-to-date based on the IdP.
func (b *bridge) Sync() error {
	// fetch current SP list
	spList, err := b.sp.List()
	if err != nil {
		return err
	}
	spDns := make([]string, len(spList))
	log.Printf("Init: sp list: %+v", spList)

	// fetch LDAP list
	idpRes, err := b.idp.Search(nil)
	if err != nil {
		return err
	}
	group := idpRes.Entries[0]
	if group == nil {
		return fmt.Errorf("LDAP search failed to find group")
	}
	memberDns := group.GetAttributeValues("member")
	log.Printf("Init: idp res: %+v", idpRes)
	idpRes.PrettyPrint(2)

	// update bridge store to reflect what's in the SP
	for _, spUser := range spList {
		dn, err := b.users.GetDN(spUser.ID)
		if err != nil {
			return err
		} else if dn == "" {
			// we don't know about this GUID yet
			idpRes, err := b.idp.FetchUID(spUser.UserName)
			if err != nil {
				return err
			}
			idpUser := idpRes[0]
			if idpUser == nil {
				// probably should clear this entry from the SP
			}
			b.users.Add(idpUser.DN, spUser)
		} else if !isMember(memberDns, dn) {
			b.Del(dn)
		} else {
			spDns = append(spDns, dn)
		}
	}

	// update the SP with what's in the IdP
	for _, memberDn := range memberDns {
		guid, err := b.users.GetGUID(memberDn)
		if err != nil {
			return err
		} else if guid == "" {
			// if we don't know about this DN already, it's not on the SP
			b.Add(memberDn)
		} else if !isMember(spDns, memberDn) {
			entry, err := b.idp.Fetch(memberDn)
			if err != nil {
				return err
			}
			user, err := b.mapEntry(entry)
			if err != nil {
				return err
			}
			b.sp.Add(user)
		}
	}

	return nil
}

func isMember(list []string, candidate string) bool {
	for _, v := range list {
		if v == candidate {
			return true
		}
	}
	return false
}

func (b *bridge) Start() {
	go b.run()
	go b.startHTTP()
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

	log.Printf("add: %s added", dn)
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

func (b *bridge) startHTTP() {
	mux := http.NewServeMux()
	mux.Handle("/_debug", b)
	l, _ := net.Listen("tcp", ":4444")
	defer l.Close()
	srv := http.Server{
		Handler: mux,
	}
	log.Println("listening for web on :4444")
	srv.Serve(l)
}

func (b *bridge) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log.Println("HTTP debug request")

	list, err := b.users.List()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "oops: %s", err)
	}

	buf, err := json.Marshal(list)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "oops: %s", err)
	}
	fmt.Fprintf(w, "%s", buf)
}

type ldapConfig struct {
	addr   string
	bindDn string
	bindPw string
	baseDn string
	group  string
}

type config struct {
	ldap   ldapConfig
	dbPath string
}

func loadConfig() config {
	c := config{
		ldap: ldapConfig{
			addr:   "localhost:389",
			bindDn: "cn=admin,dc=planetexpress,dc=com",
			bindPw: "GoodNewsEveryone",
			baseDn: "ou=people,dc=planetexpress,dc=com",
			group:  "idptool",
		},
		dbPath: "bridge.db",
	}
	return c
}

func main() {
	c := loadConfig()

	conn, err := ldap.Dial("tcp", c.ldap.addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err = conn.Bind(c.ldap.bindDn, c.ldap.bindPw); err != nil {
		log.Fatal(err)
	}

	db, err := bolt.Open(c.dbPath, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Search to monitor for changes
	searchRequest := ldap.NewSearchRequest(
		c.ldap.baseDn,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(cn=%s)", c.ldap.group),
		[]string{"*", "modifyTimestamp"},
		nil,
	)

	lb := idp.NewLDAPProvider(conn, searchRequest)
	sp := sp.NewSCIMProvider()
	b := newBridge(lb, sp, db)

	if err = b.Init(); err != nil {
		log.Fatal(err)
	}

	if err = b.Sync(); err != nil {
		log.Fatal(err)
	}

	// run until SIGINT is triggered
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt)

	b.Start()

	<-term
}
