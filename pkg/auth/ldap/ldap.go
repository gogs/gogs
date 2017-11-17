// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package ldap provide functions & structure to query a LDAP ldap directory
// For now, it's mainly tested again an MS Active Directory service, see README.md for more information
package ldap

import (
	"crypto/tls"
	"fmt"
	"strings"

	log "gopkg.in/clog.v1"
	"gopkg.in/ldap.v2"
)

type SecurityProtocol int

// Note: new type must be added at the end of list to maintain compatibility.
const (
	SECURITY_PROTOCOL_UNENCRYPTED SecurityProtocol = iota
	SECURITY_PROTOCOL_LDAPS
	SECURITY_PROTOCOL_START_TLS
)

// Basic LDAP authentication service
type Source struct {
	Name              string // canonical name (ie. corporate.ad)
	Host              string // LDAP host
	Port              int    // port number
	SecurityProtocol  SecurityProtocol
	SkipVerify        bool
	BindDN            string // DN to bind with
	BindPassword      string // Bind DN password
	UserBase          string // Base search path for users
	UserDN            string // Template for the DN of the user for simple auth
	AttributeUsername string // Username attribute
	AttributeName     string // First name attribute
	AttributeSurname  string // Surname attribute
	AttributeMail     string // E-mail attribute
	AttributesInBind  bool   // fetch attributes in bind context (not user)
	Filter            string // Query filter to validate entry
	AdminFilter       string // Query filter to check if user is admin
	GroupEnabled      bool   // if the group checking is enabled
	GroupDN           string // Group Search Base
	GroupFilter       string // Group Name Filter
	GroupMemberUID    string // Group Attribute containing array of UserUID
	UserUID           string // User Attribute listed in Group
	Enabled           bool   // if this source is disabled
}

func (ls *Source) sanitizedUserQuery(username string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4515
	badCharacters := "\x00()*\\"
	if strings.ContainsAny(username, badCharacters) {
		log.Trace("LDAP: Username contains invalid query characters: %s", username)
		return "", false
	}

	return fmt.Sprintf(ls.Filter, username), true
}

func (ls *Source) sanitizedUserDN(username string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4514: "special characters"
	badCharacters := "\x00()*\\,='\"#+;<>"
	if strings.ContainsAny(username, badCharacters) || strings.HasPrefix(username, " ") || strings.HasSuffix(username, " ") {
		log.Trace("LDAP: Username contains invalid query characters: %s", username)
		return "", false
	}

	return fmt.Sprintf(ls.UserDN, username), true
}

func (ls *Source) sanitizedGroupFilter(group string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4515
	badCharacters := "\x00*\\"
	if strings.ContainsAny(group, badCharacters) {
		log.Trace("LDAP: Group filter invalid query characters: %s", group)
		return "", false
	}

	return group, true
}

func (ls *Source) sanitizedGroupDN(groupDn string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4514: "special characters"
	badCharacters := "\x00()*\\'\"#+;<>"
	if strings.ContainsAny(groupDn, badCharacters) || strings.HasPrefix(groupDn, " ") || strings.HasSuffix(groupDn, " ") {
		log.Trace("LDAP: Group DN contains invalid query characters: %s", groupDn)
		return "", false
	}

	return groupDn, true
}

func (ls *Source) findUserDN(l *ldap.Conn, name string) (string, bool) {
	log.Trace("Search for LDAP user: %s", name)
	if len(ls.BindDN) > 0 && len(ls.BindPassword) > 0 {
		// Replace placeholders with username
		bindDN := strings.Replace(ls.BindDN, "%s", name, -1)
		err := l.Bind(bindDN, ls.BindPassword)
		if err != nil {
			log.Trace("LDAP: Failed to bind as BindDN '%s': %v", bindDN, err)
			return "", false
		}
		log.Trace("LDAP: Bound as BindDN: %s", bindDN)
	} else {
		log.Trace("LDAP: Proceeding with anonymous LDAP search")
	}

	// A search for the user.
	userFilter, ok := ls.sanitizedUserQuery(name)
	if !ok {
		return "", false
	}

	log.Trace("LDAP: Searching for DN using filter '%s' and base '%s'", userFilter, ls.UserBase)
	search := ldap.NewSearchRequest(
		ls.UserBase, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0,
		false, userFilter, []string{}, nil)

	// Ensure we found a user
	sr, err := l.Search(search)
	if err != nil || len(sr.Entries) < 1 {
		log.Trace("LDAP: Failed search using filter '%s': %v", userFilter, err)
		return "", false
	} else if len(sr.Entries) > 1 {
		log.Trace("LDAP: Filter '%s' returned more than one user", userFilter)
		return "", false
	}

	userDN := sr.Entries[0].DN
	if userDN == "" {
		log.Error(2, "LDAP: Search was successful, but found no DN!")
		return "", false
	}

	return userDN, true
}

func dial(ls *Source) (*ldap.Conn, error) {
	log.Trace("LDAP: Dialing with security protocol '%v' without verifying: %v", ls.SecurityProtocol, ls.SkipVerify)

	tlsCfg := &tls.Config{
		ServerName:         ls.Host,
		InsecureSkipVerify: ls.SkipVerify,
	}
	if ls.SecurityProtocol == SECURITY_PROTOCOL_LDAPS {
		return ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port), tlsCfg)
	}

	conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port))
	if err != nil {
		return nil, fmt.Errorf("Dial: %v", err)
	}

	if ls.SecurityProtocol == SECURITY_PROTOCOL_START_TLS {
		if err = conn.StartTLS(tlsCfg); err != nil {
			conn.Close()
			return nil, fmt.Errorf("StartTLS: %v", err)
		}
	}

	return conn, nil
}

func bindUser(l *ldap.Conn, userDN, passwd string) error {
	log.Trace("Binding with userDN: %s", userDN)
	err := l.Bind(userDN, passwd)
	if err != nil {
		log.Trace("LDAP authentication failed for '%s': %v", userDN, err)
		return err
	}
	log.Trace("Bound successfully with userDN: %s", userDN)
	return err
}

// searchEntry : search an LDAP source if an entry (name, passwd) is valid and in the specific filter
func (ls *Source) SearchEntry(name, passwd string, directBind bool) (string, string, string, string, bool, bool) {
	// See https://tools.ietf.org/search/rfc4513#section-5.1.2
	if len(passwd) == 0 {
		log.Trace("authentication failed for '%s' with empty password")
		return "", "", "", "", false, false
	}
	l, err := dial(ls)
	if err != nil {
		log.Error(2, "LDAP connect failed for '%s': %v", ls.Host, err)
		ls.Enabled = false
		return "", "", "", "", false, false
	}
	defer l.Close()

	var userDN string
	if directBind {
		log.Trace("LDAP will bind directly via UserDN template: %s", ls.UserDN)

		var ok bool
		userDN, ok = ls.sanitizedUserDN(name)
		if !ok {
			return "", "", "", "", false, false
		}
	} else {
		log.Trace("LDAP will use BindDN")

		var found bool
		userDN, found = ls.findUserDN(l, name)
		if !found {
			return "", "", "", "", false, false
		}
	}

	if directBind || !ls.AttributesInBind {
		// binds user (checking password) before looking-up attributes in user context
		err = bindUser(l, userDN, passwd)
		if err != nil {
			return "", "", "", "", false, false
		}
	}

	userFilter, ok := ls.sanitizedUserQuery(name)
	if !ok {
		return "", "", "", "", false, false
	}

	log.Trace("Fetching attributes '%v', '%v', '%v', '%v', '%v' with filter '%s' and base '%s'",
		ls.AttributeUsername, ls.AttributeName, ls.AttributeSurname, ls.AttributeMail, ls.UserUID, userFilter, userDN)
	search := ldap.NewSearchRequest(
		userDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, userFilter,
		[]string{ls.AttributeUsername, ls.AttributeName, ls.AttributeSurname, ls.AttributeMail, ls.UserUID},
		nil)

	sr, err := l.Search(search)
	if err != nil {
		log.Error(2, "LDAP: User search failed: %v", err)
		return "", "", "", "", false, false
	} else if len(sr.Entries) < 1 {
		if directBind {
			log.Trace("LDAP: User filter inhibited user login")
		} else {
			log.Trace("LDAP: User search failed: 0 entries")
		}

		return "", "", "", "", false, false
	}

	username := sr.Entries[0].GetAttributeValue(ls.AttributeUsername)
	firstname := sr.Entries[0].GetAttributeValue(ls.AttributeName)
	surname := sr.Entries[0].GetAttributeValue(ls.AttributeSurname)
	mail := sr.Entries[0].GetAttributeValue(ls.AttributeMail)
	uid := sr.Entries[0].GetAttributeValue(ls.UserUID)

	// Check group membership
	if ls.GroupEnabled {
		groupFilter, ok := ls.sanitizedGroupFilter(ls.GroupFilter)
		if !ok {
			return "", "", "", "", false, false
		}
		groupDN, ok := ls.sanitizedGroupDN(ls.GroupDN)
		if !ok {
			return "", "", "", "", false, false
		}

		log.Trace("LDAP: Fetching groups '%v' with filter '%s' and base '%s'", ls.GroupMemberUID, groupFilter, groupDN)
		groupSearch := ldap.NewSearchRequest(
			groupDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, groupFilter,
			[]string{ls.GroupMemberUID},
			nil)

		srg, err := l.Search(groupSearch)
		if err != nil {
			log.Error(2, "LDAP: Group search failed: %v", err)
			return "", "", "", "", false, false
		} else if len(sr.Entries) < 1 {
			log.Error(2, "LDAP: Group search failed: 0 entries")
			return "", "", "", "", false, false
		}

		isMember := false
		for _, group := range srg.Entries {
			for _, member := range group.GetAttributeValues(ls.GroupMemberUID) {
				if member == uid {
					isMember = true
				}
			}
		}

		if !isMember {
			log.Trace("LDAP: Group membership test failed [username: %s, group_member_uid: %s, user_uid: %s", username, ls.GroupMemberUID, uid)
			return "", "", "", "", false, false
		}
	}

	isAdmin := false
	if len(ls.AdminFilter) > 0 {
		log.Trace("Checking admin with filter '%s' and base '%s'", ls.AdminFilter, userDN)
		search = ldap.NewSearchRequest(
			userDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, ls.AdminFilter,
			[]string{ls.AttributeName},
			nil)

		sr, err = l.Search(search)
		if err != nil {
			log.Error(2, "LDAP: Admin search failed: %v", err)
		} else if len(sr.Entries) < 1 {
			log.Error(2, "LDAP: Admin search failed: 0 entries")
		} else {
			isAdmin = true
		}
	}

	if !directBind && ls.AttributesInBind {
		// binds user (checking password) after looking-up attributes in BindDN context
		err = bindUser(l, userDN, passwd)
		if err != nil {
			return "", "", "", "", false, false
		}
	}

	return username, firstname, surname, mail, isAdmin, true
}
