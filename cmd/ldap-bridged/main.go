package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/boltdb/bolt"

	"github.com/mtodd/ldapwatch"

	ldap "gopkg.in/ldap.v2"
)

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

func handleUpdates(c chan event, done chan struct{}) {
	for {
		select {
		case e := <-c:
			before := e.before
			after := e.after
			log.Printf("change detected: %s", after.DN)
			c := computeChanges(before, after)
			log.Printf("%+v", c)
		case <-done:
			return
		}
	}
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

	var b *bolt.Bucket
	db.Update(func(tx *bolt.Tx) error {
		b, err = tx.CreateBucketIfNotExists([]byte("users"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	log.Printf("%+v", b)

	updates := make(chan event)
	done := make(chan struct{})
	defer func() { close(done) }()
	go handleUpdates(updates, done)

	w, err := ldapwatch.NewWatcher(conn, 1*time.Second, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer w.Stop()

	c := groupMembershipChecker{
		c: updates,
	}

	// Search to monitor for changes
	searchRequest := ldap.NewSearchRequest(
		"ou=people,dc=planetexpress,dc=com",
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		// "(cn=ship_crew)",
		"(cn=test_group)",
		[]string{"*", "modifyTimestamp"},
		nil,
	)

	// register the search
	w.Add(searchRequest, &c)

	// run until SIGINT is triggered
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt)

	w.Start()

	<-term
}
