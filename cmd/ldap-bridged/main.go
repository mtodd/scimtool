package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
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
	cfg   bridgeConfig
	idp   idp.LDAPProvider
	sp    sp.SCIMProvider
	idps  []identityProviderI
	sps   []serviceProviderI
	db    *bolt.DB
	users users.Users
}

func newBridge(cfg bridgeConfig) bridge {
	idps := make([]identityProviderI, 1)
	sps := make([]serviceProviderI, 1)

	return bridge{
		cfg:  cfg,
		idps: idps,
		sps:  sps,
	}
}

func (b *bridge) Link(spi serviceProviderI) error {
	// LEGACY
	scimsp := spi.(sp.SCIMProvider)
	b.sp = scimsp

	b.sps = append(b.sps, spi)
	return nil
}

func (b *bridge) Init() error {
	db, err := bolt.Open(b.cfg.DBPath, 0600, nil)
	if err != nil {
		return fmt.Errorf("%s", err)
	}
	b.db = db

	b.users = users.New(db)
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

func (b *bridge) Start() error {
	go b.run()
	go b.startHTTP()
	return b.idp.Start()
}

func (b *bridge) Stop() {
	b.db.Close()
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
		log.Printf("add: IdP fetch(%s): %s", dn, err)
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
		return
	}

	// receive GUID
	user.ID = guid

	// persist membership
	// persist DN-to-GUID mapping
	// persist GUID-to-DN mapping
	if err = b.users.Add(dn, user); err != nil {
		log.Printf("add: bridge store failed: %s", err)
		return
	}

	log.Printf("add: %s added", dn)
}

func (b *bridge) Del(dn string) {
	log.Printf("remove: %s", dn)

	guid, err := b.users.GetGUID(dn)
	if err != nil {
		log.Printf("remove: get guid(%s): %s", dn, err)
		return
	}

	if err := b.sp.Del(guid); err != nil {
		log.Printf("remove: %s failed: %s", guid, err)
		return
	}

	if err = b.users.Del(guid, dn); err != nil {
		log.Printf("remove: bridge store failed: %s", err)
		return
	}
}

// mapEntry takes an LDAP entry, maps to a SCIM user representation
func (b *bridge) mapEntry(entry *ldap.Entry) (scim.User, error) {
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

type scimConfig struct {
	org    string
	token  string
	dryRun bool
}

type config struct {
	ldap   ldapConfig
	scim   scimConfig
	DBPath string
}

type serviceProviderConfig struct {
	Adapter string                 `json:"adapter"`
	Config  map[string]interface{} `json:"config"`
}
type identityProviderConfig struct {
	Adapter          string                  `json:"adapter"`
	Config           map[string]interface{}  `json:"config"`
	ServiceProviders []serviceProviderConfig `json:"serviceProviders"`
}
type bridgeConfig struct {
	DBPath            string                   `json:"dbPath"`
	IdentityProviders []identityProviderConfig `json:"identityProviders"`
}

func loadConfigFile(c *bridgeConfig, path string) error {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	log.Println(string(dat))

	err = json.Unmarshal(dat, &c)
	if err != nil {
		return err
	}

	log.Printf("%#v", c)

	return nil
}

func loadConfig() bridgeConfig {
	c := bridgeConfig{}

	configPath := flag.String("config", "", "specify a configuration file to load")

	flag.Parse()

	if len(*configPath) > 0 {
		if err := loadConfigFile(&c, *configPath); err != nil {
			panic(err)
		}
	}

	// if addr := os.Getenv("LDAP_ADDR"); addr != "" {
	// 	c.ldap.addr = addr
	// }
	// if bindDn := os.Getenv("LDAP_BIND"); bindDn != "" {
	// 	c.ldap.bindDn = bindDn
	// }
	// if bindPw := os.Getenv("LDAP_PASS"); bindPw != "" {
	// 	c.ldap.bindPw = bindPw
	// }
	// if baseDn := os.Getenv("LDAP_BASE"); baseDn != "" {
	// 	c.ldap.baseDn = baseDn
	// }
	// if group := os.Getenv("LDAP_GROUP"); group != "" {
	// 	c.ldap.group = group
	// }
	//
	// if org := os.Getenv("SCIM_ORG"); org != "" {
	// 	c.scim.org = org
	// }
	// if token := os.Getenv("SCIM_TOKEN"); token != "" {
	// 	c.scim.token = token
	// }
	// if dryRun := os.Getenv("SCIM_DRY"); dryRun != "" {
	// 	c.scim.dryRun = dryRun != "false"
	// }
	//
	// if dbPath := os.Getenv("DB"); dbPath != "" {
	// 	c.dbPath = dbPath
	// }

	return c
}

type identityProvider struct {
	cfg map[string]interface{}
	sps []serviceProvider

	spsi []serviceProviderI
}

type serviceProvider struct {
	cfg map[string]interface{}
}

type identityProviderI interface{}
type serviceProviderI interface{}

func main() {
	var err error

	cfg := loadConfig()

	if len(cfg.IdentityProviders) == 0 {
		log.Fatalf("config: identity provider required")
	}

	log.Printf("%#v", cfg)

	b := newBridge(cfg)

	for _, idpCfg := range cfg.IdentityProviders {
		switch idpCfg.Adapter {
		case "ldap":
			lb := idp.NewLDAPProvider(idpCfg.Config)
			log.Printf("loading LDAP provider: %#v", lb)

			b.idp = lb
			b.idps = append(b.idps, lb)

			if len(idpCfg.ServiceProviders) == 0 {
				log.Fatalf("config: service provider required for %s identity provider", idpCfg.Adapter)
			}

			for _, spCfg := range idpCfg.ServiceProviders {
				sp := sp.NewSCIMProvider(spCfg.Config)
				if err = b.Link(sp); err != nil {
					log.Fatalf("config: service provider: link: %s", err)
				}
			}
		default:
			log.Fatalf("loadConfig: unrecognized IdP adapter: %s", idpCfg.Adapter)
		}
	}

	if err = b.Init(); err != nil {
		log.Fatalf("bridge: init: %s", err)
	}

	if err = b.Start(); err != nil {
		log.Fatalf("bridge: start: %s", err)
	}
	defer b.Stop()

	if err = b.Sync(); err != nil {
		log.Fatalf("bridge: sync: %s", err)
	}

	// run until SIGINT is triggered
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt)
	<-term
}
