// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package ldap provide functions & structure to query a LDAP ldap directory.
// For now, it's mainly tested again an MS Active Directory service, see README.md for more information.
package ldap

import (
	"crypto/tls"
	"fmt"
	"strings"

	"gopkg.in/ldap.v2"
	log "unknwon.dev/clog/v2"
)

// SecurityProtocol is the security protocol when the authenticate provider talks to LDAP directory.
type SecurityProtocol int

// ⚠️ WARNING: new type must be added at the end of list to maintain compatibility.
const (
	SecurityProtocolUnencrypted SecurityProtocol = iota
	SecurityProtocolLDAPS
	SecurityProtocolStartTLS
)

// SecurityProtocolName returns the human-readable name for given security protocol.
func SecurityProtocolName(protocol SecurityProtocol) string {
	return map[SecurityProtocol]string{
		SecurityProtocolUnencrypted: "Unencrypted",
		SecurityProtocolLDAPS:       "LDAPS",
		SecurityProtocolStartTLS:    "StartTLS",
	}[protocol]
}

// Config contains configuration for LDAP authentication.
//
// ⚠️ WARNING: Change to the field name must preserve the INI key name for backward compatibility.
type Config struct {
	Host              string // LDAP host
	Port              int    // Port number
	SecurityProtocol  SecurityProtocol
	SkipVerify        bool
	BindDN            string `ini:"bind_dn,omitempty"` // DN to bind with
	BindPassword      string `ini:",omitempty"`        // Bind DN password
	UserBase          string `ini:",omitempty"`        // Base search path for users
	UserDN            string `ini:"user_dn,omitempty"` // Template for the DN of the user for simple auth
	AttributeUsername string // Username attribute
	AttributeName     string // First name attribute
	AttributeSurname  string // Surname attribute
	AttributeMail     string // Email attribute
	AttributesInBind  bool   // Fetch attributes in bind context (not user)
	Filter            string // Query filter to validate entry
	AdminFilter       string // Query filter to check if user is admin
	GroupEnabled      bool   // Whether the group checking is enabled
	GroupDN           string `ini:"group_dn"` // Group search base
	GroupFilter       string // Group name filter
	GroupMemberUID    string `ini:"group_member_uid"` // Group Attribute containing array of UserUID
	UserUID           string `ini:"user_uid"`         // User Attribute listed in group
}

func (c *Config) SecurityProtocolName() string {
	return SecurityProtocolName(c.SecurityProtocol)
}

func (c *Config) sanitizedUserQuery(username string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4515
	badCharacters := "\x00()*\\"
	if strings.ContainsAny(username, badCharacters) {
		log.Trace("LDAP: Username contains invalid query characters: %s", username)
		return "", false
	}

	return strings.Replace(c.Filter, "%s", username, -1), true
}

func (c *Config) sanitizedUserDN(username string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4514: "special characters"
	badCharacters := "\x00()*\\,='\"#+;<>"
	if strings.ContainsAny(username, badCharacters) || strings.HasPrefix(username, " ") || strings.HasSuffix(username, " ") {
		log.Trace("LDAP: Username contains invalid query characters: %s", username)
		return "", false
	}

	return strings.Replace(c.UserDN, "%s", username, -1), true
}

func (c *Config) sanitizedGroupFilter(group string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4515
	badCharacters := "\x00*\\"
	if strings.ContainsAny(group, badCharacters) {
		log.Trace("LDAP: Group filter invalid query characters: %s", group)
		return "", false
	}

	return group, true
}

func (c *Config) sanitizedGroupDN(groupDn string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4514: "special characters"
	badCharacters := "\x00()*\\'\"#+;<>"
	if strings.ContainsAny(groupDn, badCharacters) || strings.HasPrefix(groupDn, " ") || strings.HasSuffix(groupDn, " ") {
		log.Trace("LDAP: Group DN contains invalid query characters: %s", groupDn)
		return "", false
	}

	return groupDn, true
}

func (c *Config) findUserDN(l *ldap.Conn, name string) (string, bool) {
	log.Trace("Search for LDAP user: %s", name)
	if len(c.BindDN) > 0 && len(c.BindPassword) > 0 {
		// Replace placeholders with username
		bindDN := strings.Replace(c.BindDN, "%s", name, -1)
		err := l.Bind(bindDN, c.BindPassword)
		if err != nil {
			log.Trace("LDAP: Failed to bind as BindDN '%s': %v", bindDN, err)
			return "", false
		}
		log.Trace("LDAP: Bound as BindDN: %s", bindDN)
	} else {
		log.Trace("LDAP: Proceeding with anonymous LDAP search")
	}

	// A search for the user.
	userFilter, ok := c.sanitizedUserQuery(name)
	if !ok {
		return "", false
	}

	log.Trace("LDAP: Searching for DN using filter %q and base %q", userFilter, c.UserBase)
	search := ldap.NewSearchRequest(
		c.UserBase, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0,
		false, userFilter, []string{}, nil)

	// Ensure we found a user
	sr, err := l.Search(search)
	if err != nil || len(sr.Entries) < 1 {
		log.Trace("LDAP: Failed to search using filter %q: %v", userFilter, err)
		return "", false
	} else if len(sr.Entries) > 1 {
		log.Trace("LDAP: Filter %q returned more than one user", userFilter)
		return "", false
	}

	userDN := sr.Entries[0].DN
	if userDN == "" {
		log.Error("LDAP: Search was successful, but found no DN!")
		return "", false
	}

	return userDN, true
}

func dial(ls *Config) (*ldap.Conn, error) {
	log.Trace("LDAP: Dialing with security protocol '%v' without verifying: %v", ls.SecurityProtocol, ls.SkipVerify)

	tlsCfg := &tls.Config{
		ServerName:         ls.Host,
		InsecureSkipVerify: ls.SkipVerify,
	}
	if ls.SecurityProtocol == SecurityProtocolLDAPS {
		return ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port), tlsCfg)
	}

	conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port))
	if err != nil {
		return nil, fmt.Errorf("Dial: %v", err)
	}

	if ls.SecurityProtocol == SecurityProtocolStartTLS {
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

// searchEntry searches an LDAP source if an entry (name, passwd) is valid and in the specific filter.
func (c *Config) searchEntry(name, passwd string, directBind bool) (string, string, string, string, bool, bool) {
	// See https://tools.ietf.org/search/rfc4513#section-5.1.2
	if len(passwd) == 0 {
		log.Trace("authentication failed for '%s' with empty password", name)
		return "", "", "", "", false, false
	}
	l, err := dial(c)
	if err != nil {
		log.Error("LDAP connect failed for '%s': %v", c.Host, err)
		return "", "", "", "", false, false
	}
	defer l.Close()

	var userDN string
	if directBind {
		log.Trace("LDAP will bind directly via UserDN template: %s", c.UserDN)

		var ok bool
		userDN, ok = c.sanitizedUserDN(name)
		if !ok {
			return "", "", "", "", false, false
		}
	} else {
		log.Trace("LDAP will use BindDN")

		var found bool
		userDN, found = c.findUserDN(l, name)
		if !found {
			return "", "", "", "", false, false
		}
	}

	if directBind || !c.AttributesInBind {
		// binds user (checking password) before looking-up attributes in user context
		err = bindUser(l, userDN, passwd)
		if err != nil {
			return "", "", "", "", false, false
		}
	}

	userFilter, ok := c.sanitizedUserQuery(name)
	if !ok {
		return "", "", "", "", false, false
	}

	log.Trace("Fetching attributes %q, %q, %q, %q, %q with user filter %q and user DN %q",
		c.AttributeUsername, c.AttributeName, c.AttributeSurname, c.AttributeMail, c.UserUID, userFilter, userDN)

	search := ldap.NewSearchRequest(
		userDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, userFilter,
		[]string{c.AttributeUsername, c.AttributeName, c.AttributeSurname, c.AttributeMail, c.UserUID},
		nil)
	sr, err := l.Search(search)
	if err != nil {
		log.Error("LDAP: User search failed: %v", err)
		return "", "", "", "", false, false
	} else if len(sr.Entries) < 1 {
		if directBind {
			log.Trace("LDAP: User filter inhibited user login")
		} else {
			log.Trace("LDAP: User search failed: 0 entries")
		}

		return "", "", "", "", false, false
	}

	username := sr.Entries[0].GetAttributeValue(c.AttributeUsername)
	firstname := sr.Entries[0].GetAttributeValue(c.AttributeName)
	surname := sr.Entries[0].GetAttributeValue(c.AttributeSurname)
	mail := sr.Entries[0].GetAttributeValue(c.AttributeMail)
	uid := sr.Entries[0].GetAttributeValue(c.UserUID)

	// Check group membership
	if c.GroupEnabled {
		groupFilter, ok := c.sanitizedGroupFilter(c.GroupFilter)
		if !ok {
			return "", "", "", "", false, false
		}
		groupDN, ok := c.sanitizedGroupDN(c.GroupDN)
		if !ok {
			return "", "", "", "", false, false
		}

		log.Trace("LDAP: Fetching groups '%v' with filter '%s' and base '%s'", c.GroupMemberUID, groupFilter, groupDN)
		groupSearch := ldap.NewSearchRequest(
			groupDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, groupFilter,
			[]string{c.GroupMemberUID},
			nil)

		srg, err := l.Search(groupSearch)
		if err != nil {
			log.Error("LDAP: Group search failed: %v", err)
			return "", "", "", "", false, false
		} else if len(srg.Entries) < 1 {
			log.Trace("LDAP: Group search returned no entries")
			return "", "", "", "", false, false
		}

		isMember := false
		if c.UserUID == "dn" {
			for _, group := range srg.Entries {
				for _, member := range group.GetAttributeValues(c.GroupMemberUID) {
					if member == sr.Entries[0].DN {
						isMember = true
					}
				}
			}
		} else {
			for _, group := range srg.Entries {
				for _, member := range group.GetAttributeValues(c.GroupMemberUID) {
					if member == uid {
						isMember = true
					}
				}
			}
		}

		if !isMember {
			log.Trace("LDAP: Group membership test failed [username: %s, group_member_uid: %s, user_uid: %s", username, c.GroupMemberUID, uid)
			return "", "", "", "", false, false
		}
	}

	isAdmin := false
	if len(c.AdminFilter) > 0 {
		log.Trace("Checking admin with filter '%s' and base '%s'", c.AdminFilter, userDN)
		search = ldap.NewSearchRequest(
			userDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, c.AdminFilter,
			[]string{c.AttributeName},
			nil)

		sr, err = l.Search(search)
		if err != nil {
			log.Error("LDAP: Admin search failed: %v", err)
		} else if len(sr.Entries) < 1 {
			log.Trace("LDAP: Admin search returned no entries")
		} else {
			isAdmin = true
		}
	}

	if !directBind && c.AttributesInBind {
		// binds user (checking password) after looking-up attributes in BindDN context
		err = bindUser(l, userDN, passwd)
		if err != nil {
			return "", "", "", "", false, false
		}
	}

	return username, firstname, surname, mail, isAdmin, true
}
