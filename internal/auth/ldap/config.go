// Package ldap provide functions & structure to query a LDAP ldap directory.
// For now, it's mainly tested again an MS Active Directory service, see README.md for more information.
package ldap

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/go-ldap/ldap/v3"
	"unknwon.dev/x/logx"
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

func (c *Config) sanitizedUserQuery(logger *logx.Logger, username string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4515
	badCharacters := "\x00()*\\"
	if strings.ContainsAny(username, badCharacters) {
		logger.Debug("Username contains invalid query characters", "username", username)
		return "", false
	}

	return strings.ReplaceAll(c.Filter, "%s", username), true
}

func (c *Config) sanitizedUserDN(logger *logx.Logger, username string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4514: "special characters"
	badCharacters := "\x00()*\\,='\"#+;<>"
	if strings.ContainsAny(username, badCharacters) || strings.HasPrefix(username, " ") || strings.HasSuffix(username, " ") {
		logger.Debug("Username contains invalid query characters", "username", username)
		return "", false
	}

	return strings.ReplaceAll(c.UserDN, "%s", username), true
}

func (*Config) sanitizedGroupFilter(logger *logx.Logger, group string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4515
	badCharacters := "\x00*\\"
	if strings.ContainsAny(group, badCharacters) {
		logger.Debug("Group filter contains invalid query characters", "groupFilter", group)
		return "", false
	}

	return group, true
}

func (*Config) sanitizedGroupDN(logger *logx.Logger, groupDn string) (string, bool) {
	// See http://tools.ietf.org/search/rfc4514: "special characters"
	badCharacters := "\x00()*\\'\"#+;<>"
	if strings.ContainsAny(groupDn, badCharacters) || strings.HasPrefix(groupDn, " ") || strings.HasSuffix(groupDn, " ") {
		logger.Debug("Group DN contains invalid query characters", "groupDN", groupDn)
		return "", false
	}

	return groupDn, true
}

func (c *Config) findUserDN(logger *logx.Logger, l *ldap.Conn, name string) (string, bool) {
	logger.Debug("Searching for user", "username", name)
	if len(c.BindDN) > 0 && len(c.BindPassword) > 0 {
		// Replace placeholders with username
		bindDN := strings.ReplaceAll(c.BindDN, "%s", name)
		err := l.Bind(bindDN, c.BindPassword)
		if err != nil {
			logger.Debug("Failed to bind as bind DN", "bindDN", bindDN, "error", err)
			return "", false
		}
		logger.Debug("Bound as bind DN", "bindDN", bindDN)
	} else {
		logger.Debug("Proceeding with anonymous search")
	}

	// A search for the user.
	userFilter, ok := c.sanitizedUserQuery(logger, name)
	if !ok {
		return "", false
	}

	logger.Debug("Searching for user DN", "userFilter", userFilter, "userBase", c.UserBase)
	search := ldap.NewSearchRequest(
		c.UserBase, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0,
		false, userFilter, []string{}, nil)

	// Ensure we found a user
	sr, err := l.Search(search)
	if err != nil || len(sr.Entries) < 1 {
		logger.Debug("Failed to search for user DN", "userFilter", userFilter, "error", err)
		return "", false
	} else if len(sr.Entries) > 1 {
		logger.Debug("User filter returned more than one user", "userFilter", userFilter)
		return "", false
	}

	userDN := sr.Entries[0].DN
	if userDN == "" {
		logger.Error("Search succeeded but found no user DN", "userFilter", userFilter)
		return "", false
	}

	return userDN, true
}

func dial(logger *logx.Logger, ls *Config) (*ldap.Conn, error) {
	logger.Debug("Dialing server", "host", ls.Host, "securityProtocol", ls.SecurityProtocol, "skipVerify", ls.SkipVerify)

	tlsCfg := &tls.Config{
		ServerName:         ls.Host,
		InsecureSkipVerify: ls.SkipVerify,
	}
	if ls.SecurityProtocol == SecurityProtocolLDAPS {
		return ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port), tlsCfg)
	}

	conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port))
	if err != nil {
		return nil, errors.Newf("dial: %v", err)
	}

	if ls.SecurityProtocol == SecurityProtocolStartTLS {
		if err = conn.StartTLS(tlsCfg); err != nil {
			conn.Close()
			return nil, errors.Newf("StartTLS: %v", err)
		}
	}

	return conn, nil
}

func bindUser(logger *logx.Logger, l *ldap.Conn, userDN, passwd string) error {
	logger.Debug("Binding as user", "userDN", userDN)
	err := l.Bind(userDN, passwd)
	if err != nil {
		logger.Debug("Failed to bind as user", "userDN", userDN, "error", err)
		return err
	}
	logger.Debug("Bound as user", "userDN", userDN)
	return err
}

// searchEntry searches an LDAP source if an entry (name, passwd) is valid and in the specific filter.
func (c *Config) searchEntry(logger *logx.Logger, name, passwd string, directBind bool) (string, string, string, string, bool, bool) {
	// See https://tools.ietf.org/search/rfc4513#section-5.1.2
	if passwd == "" {
		logger.Debug("Authentication failed with empty password", "username", name)
		return "", "", "", "", false, false
	}
	l, err := dial(logger, c)
	if err != nil {
		logger.Error("Failed to connect to server", "host", c.Host, "error", err)
		return "", "", "", "", false, false
	}
	defer l.Close()

	var userDN string
	if directBind {
		logger.Debug("Binding directly via user DN template", "userDNTemplate", c.UserDN)

		var ok bool
		userDN, ok = c.sanitizedUserDN(logger, name)
		if !ok {
			return "", "", "", "", false, false
		}
	} else {
		logger.Debug("Searching for user DN via bind DN")

		var found bool
		userDN, found = c.findUserDN(logger, l, name)
		if !found {
			return "", "", "", "", false, false
		}
	}

	if directBind || !c.AttributesInBind {
		// binds user (checking password) before looking-up attributes in user context
		err = bindUser(logger, l, userDN, passwd)
		if err != nil {
			return "", "", "", "", false, false
		}
	}

	userFilter, ok := c.sanitizedUserQuery(logger, name)
	if !ok {
		return "", "", "", "", false, false
	}

	logger.Debug("Fetching user attributes", "attributeUsername", c.AttributeUsername, "attributeName", c.AttributeName, "attributeSurname", c.AttributeSurname, "attributeMail", c.AttributeMail, "userUID", c.UserUID, "userFilter", userFilter, "userDN", userDN)

	search := ldap.NewSearchRequest(
		userDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, userFilter,
		[]string{c.AttributeUsername, c.AttributeName, c.AttributeSurname, c.AttributeMail, c.UserUID},
		nil)
	sr, err := l.Search(search)
	if err != nil {
		logger.Error("Failed to search for user", "error", err)
		return "", "", "", "", false, false
	} else if len(sr.Entries) < 1 {
		if directBind {
			logger.Debug("User filter inhibited user login")
		} else {
			logger.Debug("User search returned no entries")
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
		groupFilter, ok := c.sanitizedGroupFilter(logger, c.GroupFilter)
		if !ok {
			return "", "", "", "", false, false
		}
		groupDN, ok := c.sanitizedGroupDN(logger, c.GroupDN)
		if !ok {
			return "", "", "", "", false, false
		}

		logger.Debug("Fetching groups", "groupMemberUID", c.GroupMemberUID, "groupFilter", groupFilter, "groupDN", groupDN)
		groupSearch := ldap.NewSearchRequest(
			groupDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, groupFilter,
			[]string{c.GroupMemberUID},
			nil)

		srg, err := l.Search(groupSearch)
		if err != nil {
			logger.Error("Failed to search for groups", "error", err)
			return "", "", "", "", false, false
		} else if len(srg.Entries) < 1 {
			logger.Debug("Group search returned no entries")
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
			logger.Debug("Group membership test failed", "username", username, "groupMemberUID", c.GroupMemberUID, "userUID", uid)
			return "", "", "", "", false, false
		}
	}

	isAdmin := false
	if len(c.AdminFilter) > 0 {
		logger.Debug("Checking admin", "adminFilter", c.AdminFilter, "userDN", userDN)
		search = ldap.NewSearchRequest(
			userDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, c.AdminFilter,
			[]string{c.AttributeName},
			nil)

		sr, err = l.Search(search)
		if err != nil {
			logger.Error("Failed to search for admin", "error", err)
		} else if len(sr.Entries) < 1 {
			logger.Debug("Admin search returned no entries")
		} else {
			isAdmin = true
		}
	}

	if !directBind && c.AttributesInBind {
		// binds user (checking password) after looking-up attributes in BindDN context
		err = bindUser(logger, l, userDN, passwd)
		if err != nil {
			return "", "", "", "", false, false
		}
	}

	return username, firstname, surname, mail, isAdmin, true
}
