package idp

import (
	"fmt"
	"log"
	"time"

	"github.com/mtodd/ldapwatch"

	ldap "gopkg.in/ldap.v2"
)

// LDAPProvider ...
type LDAPProvider struct {
	conn    *ldap.Conn
	sr      *ldap.SearchRequest
	Added   chan string
	Removed chan string
	done    chan struct{}
}

// NewLDAPProvider ...
func NewLDAPProvider(conn *ldap.Conn, sr *ldap.SearchRequest) LDAPProvider {
	return LDAPProvider{
		conn:    conn,
		sr:      sr,
		Added:   make(chan string),
		Removed: make(chan string),
		done:    make(chan struct{}),
	}
}

// Start ...
func (p *LDAPProvider) Start() error {
	updates := make(chan event)
	done := make(chan struct{})
	// defer func() { close(done) }()
	go handleUpdates(p, updates, done)

	w, err := ldapwatch.NewWatcher(p.conn, 1*time.Second, nil)
	if err != nil {
		log.Fatal(err)
	}
	// defer w.Stop()

	c := groupMembershipChecker{
		c: updates,
	}

	// register the search
	w.Add(p.sr, &c)

	w.Start()

	return nil
}

type event struct {
	before *ldap.Entry
	after  *ldap.Entry
}

// Implements the ldapwatch.Checker interface in order to check whether
// the search results change over time.
//
// In this case, our Checker keeps track of previous results as well as
// holding a channel that we notify whenever changes are detected.
type groupMembershipChecker struct {
	prev *ldap.SearchResult
	c    chan event
}

// Check receives the result of the search; the Checker needs to take action
// if the result does not match what it expects.
func (c *groupMembershipChecker) Check(r *ldap.SearchResult, err error) {
	if err != nil {
		log.Printf("%s", err)
		return
	}

	// first search sets baseline
	if c.prev == nil {
		c.prev = r
		r.PrettyPrint(2)
		return
	}

	if len(c.prev.Entries) != len(r.Entries) {
		// entries returned does not match
		c.prev = r
		return
	}

	prevEntry := c.prev.Entries[0]
	nextEntry := r.Entries[0]

	if prevEntry.GetAttributeValue("modifyTimestamp") != nextEntry.GetAttributeValue("modifyTimestamp") {
		// modifyTimestamp changed
		c.prev = r
		c.c <- event{prevEntry, nextEntry}
		return
	}

	// no change
}

type changes struct {
	added   []string
	removed []string
}

func computeChanges(before *ldap.Entry, after *ldap.Entry) changes {
	c := changes{}

	bs := make(map[string]bool, len(before.GetAttributeValues("member")))
	as := make(map[string]bool, len(after.GetAttributeValues("member")))

	for _, dn := range before.GetAttributeValues("member") {
		bs[dn] = true
	}
	for _, dn := range after.GetAttributeValues("member") {
		as[dn] = true
	}

	added := make(map[string]bool, len(before.GetAttributeValues("member")))
	removed := make(map[string]bool, len(after.GetAttributeValues("member")))

	for dn := range as {
		// everything in the after list could've been added
		added[dn] = true
	}
	for dn := range bs {
		// it wasn't added if it was in the before list, so remove it
		delete(added, dn)
	}

	for dn := range added {
		c.added = append(c.added, dn)
	}

	for dn := range bs {
		// everything in the before list could've been removed
		removed[dn] = true
	}
	for dn := range as {
		// it wasn't removed if it was in the after list, so remove it
		delete(removed, dn)
	}

	for dn := range removed {
		c.removed = append(c.removed, dn)
	}

	return c
}

func handleUpdates(p *LDAPProvider, c chan event, done chan struct{}) {
	for {
		select {
		case e := <-c:
			before := e.before
			after := e.after
			log.Printf("change detected: %s", after.DN)
			c := computeChanges(before, after)
			log.Printf("%+v", c)
			for _, dn := range c.added {
				p.Added <- dn
			}
			for _, dn := range c.removed {
				p.Removed <- dn
			}
		case <-done:
			return
		}
	}
}

// Fetch ...
func (p *LDAPProvider) Fetch(dn string) (*ldap.Entry, error) {
	req := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=*)",
		[]string{"dn", "uid", "cn", "sn", "givenName", "mail", "modifyTimestamp"},
		nil,
	)

	res, err := p.conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %s", err)
	}

	return res.Entries[0], nil
}

// FetchUid ...
func (p *LDAPProvider) FetchUID(uids ...string) ([]*ldap.Entry, error) {
	filter := fmt.Sprintf("(uid=%s)", uids[0])
	req := ldap.NewSearchRequest(
		p.sr.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=*)(%s))", filter),
		[]string{"dn", "uid", "cn", "sn", "givenName", "mail", "modifyTimestamp"},
		nil,
	)

	res, err := p.conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("fetch by UID (%s) failed: %s", uids, err)
	}

	return res.Entries, nil
}

// Search ...
func (p *LDAPProvider) Search(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if req == nil {
		req = p.sr
	}
	return p.conn.Search(req)
}
