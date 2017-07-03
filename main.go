package main

import (
	"fmt"
	"log"

	"github.com/mtodd/ldapwatch"

	ldap "gopkg.in/ldap.v2"
)

var (
	username = "mtodd"
	password = "passworD1"

	bindusername = "uid=admin,ou=users,dc=github,dc=com"
	bindpassword = "secret"

	base = "ou=users,dc=github,dc=com"

	host = "localhost"
	port = 3899 // 389

	watcher *ldapwatch.Watcher
)

func main() {
	watcher, err := ldapwatch.NewWatcher()

	watcher.Connect(host, port)
	defer watcher.Stop()

	watcher.Bind(bindusername, bindpassword)

	err = watcher.Add("uid=defunkt,ou=users,dc=github,dc=com")
	if err != nil {
		log.Fatal(err)
	}

	watcher.Start()
}

func other() {
	l, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	err = l.Bind(bindusername, bindpassword)
	if err != nil {
		log.Fatal(err)
	}

	// Reconnect with TLS
	// err = l.StartTLS(&tls.Config{InsecureSkipVerify: true})
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// Search for the given username
	searchRequest := ldap.NewSearchRequest(
		base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		// fmt.Sprintf("(&(objectClass=organizationalPerson)&(uid=%s))", username),
		fmt.Sprintf("(uid=%s)", username),
		[]string{"dn"},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		log.Fatal(err)
	}

	if len(sr.Entries) != 1 {
		log.Fatal("User does not exist or too many entries returned")
	}

	userdn := sr.Entries[0].DN

	// Bind as the user to verify their password
	err = l.Bind(userdn, password)
	if err != nil {
		log.Fatal(err)
	}

	// Rebind as the read only user for any futher queries
	err = l.Bind(bindusername, bindpassword)
	if err != nil {
		log.Fatal(err)
	}
}
