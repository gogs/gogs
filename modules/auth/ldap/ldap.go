// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package ldap provide functions & structure to query a LDAP ldap directory
// For now, it's mainly tested again an MS Active Directory service, see README.md for more information
package ldap

import (
	"fmt"

	"github.com/gogits/gogs/modules/ldap"
	"github.com/gogits/gogs/modules/log"
)

// Basic LDAP authentication service
type Ldapsource struct {
	Name             string // canonical name (ie. corporate.ad)
	Host             string // LDAP host
	Port             int    // port number
	UseSSL           bool   // Use SSL
	BindDN           string // DN to bind with
	BindPassword     string // Bind DN password
	UserBase         string // Base search path for users
	AttributeName    string // First name attribute
	AttributeSurname string // Surname attribute
	AttributeMail    string // E-mail attribute
	Filter           string // Query filter to validate entry
	Enabled          bool   // if this source is disabled
}

func (ls Ldapsource) FindUserDN(name string) (string, bool) {
	l, err := ldapDial(ls)
	if err != nil {
		log.Error(4, "LDAP Connect error, %s:%v", ls.Host, err)
		ls.Enabled = false
		return "", false
	}
	defer l.Close()

	log.Trace("Search for LDAP user: %s", name)
	if ls.BindDN != "" && ls.BindPassword != "" {
		err = l.Bind(ls.BindDN, ls.BindPassword)
		if err != nil {
			log.Debug("Failed to bind as BindDN: %s, %s", ls.BindDN, err.Error())
			return "", false
		}
		log.Trace("Bound as BindDN %s", ls.BindDN)
	} else {
		log.Trace("Proceeding with anonymous LDAP search.")
	}

	// A search for the user.
	userFilter := fmt.Sprintf(ls.Filter, name)
	log.Trace("Searching using filter %s", userFilter)
	search := ldap.NewSearchRequest(
		ls.UserBase, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0,
		false, userFilter, []string{}, nil)

	// Ensure we found a user
	sr, err := l.Search(search)
	if err != nil || len(sr.Entries) < 1 {
		log.Debug("Failed search using filter %s: %s", userFilter, err.Error())
		return "", false
	} else if len(sr.Entries) > 1 {
		log.Debug("Filter '%s' returned more than one user.", userFilter)
		return "", false
	}

	userDN := sr.Entries[0].DN
	if userDN == "" {
		log.Error(4, "LDAP search was succesful, but found no DN!")
		return "", false
	}

	return userDN, true
}

// searchEntry : search an LDAP source if an entry (name, passwd) is valid and in the specific filter
func (ls Ldapsource) SearchEntry(name, passwd string) (string, string, string, bool) {
	userDN, found := ls.FindUserDN(name)
	if !found {
		return "", "", "", false
	}

	l, err := ldapDial(ls)
	if err != nil {
		log.Error(4, "LDAP Connect error, %s:%v", ls.Host, err)
		ls.Enabled = false
		return "", "", "", false
	}

	defer l.Close()

	log.Trace("Binding with userDN: %s", userDN)
	err = l.Bind(userDN, passwd)
	if err != nil {
		log.Debug("LDAP auth. failed for %s, reason: %s", userDN, err.Error())
		return "", "", "", false
	}

	log.Trace("Bound successfully with userDN: %s", userDN)
	userFilter := fmt.Sprintf(ls.Filter, name)
	search := ldap.NewSearchRequest(
		userDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, userFilter,
		[]string{ls.AttributeName, ls.AttributeSurname, ls.AttributeMail},
		nil)

	sr, err := l.Search(search)
	if err != nil {
		log.Error(4, "LDAP Search failed unexpectedly! (%s)", err.Error())
		return "", "", "", false
	} else if len(sr.Entries) < 1 {
		log.Error(4, "LDAP Search failed unexpectedly! (0 entries)")
		return "", "", "", false
	}

	name_attr := sr.Entries[0].GetAttributeValue(ls.AttributeName)
	sn_attr := sr.Entries[0].GetAttributeValue(ls.AttributeSurname)
	mail_attr := sr.Entries[0].GetAttributeValue(ls.AttributeMail)
	return name_attr, sn_attr, mail_attr, true
}

func ldapDial(ls Ldapsource) (*ldap.Conn, error) {
	if ls.UseSSL {
		log.Debug("Using TLS for LDAP")
		return ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port), nil)
	} else {
		return ldap.Dial("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port))
	}
}
