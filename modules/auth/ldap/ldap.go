// Copyright github.com/juju2013. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// package ldap provide functions & structure to query a LDAP ldap directory
// For now, it's mainly tested again an MS Active Directory service, see README.md for more information
package ldap

import (
	"fmt"

	"github.com/gogits/gogs/modules/log"
	goldap "github.com/juju2013/goldap"
)

// Basic LDAP authentication service
type Ldapsource struct {
	Name         string // canonical name (ie. corporate.ad)
	Host         string // LDAP host
	Port         int    // port number
	UseSSL       bool   // Use SSL
	BaseDN       string // Base DN
	Attributes   string // Attribut to search
	Filter       string // Query filter to validate entry
	MsAdSAFormat string // in the case of MS AD Simple Authen, the format to use (see: http://msdn.microsoft.com/en-us/library/cc223499.aspx)
	Enabled      bool   // if this source is disabled
}

//Global LDAP directory pool
var (
	Authensource []Ldapsource
)

// Add a new source (LDAP directory) to the global pool
func AddSource(name string, host string, port int, usessl bool, basedn string, attributes string, filter string, msadsaformat string) {
	ldaphost := Ldapsource{name, host, port, usessl, basedn, attributes, filter, msadsaformat, true}
	Authensource = append(Authensource, ldaphost)
}

//LoginUser : try to login an user to LDAP sources, return requested (attribut,true) if ok, ("",false) other wise
//First match wins
//Returns first attribute if exists
func LoginUser(name, passwd string) (a string, r bool) {
	r = false
	for _, ls := range Authensource {
		a, r = ls.SearchEntry(name, passwd)
		if r {
			return
		}
	}
	return
}

// searchEntry : search an LDAP source if an entry (name, passwd) is valide and in the specific filter
func (ls Ldapsource) SearchEntry(name, passwd string) (string, bool) {
	l, err := ldapDial(ls)
	if err != nil {
		log.Error("LDAP Connect error, %s:%v", ls.Host, err)
		ls.Enabled = false
		return "", false
	}
	defer l.Close()

	nx := fmt.Sprintf(ls.MsAdSAFormat, name)
	err = l.Bind(nx, passwd)
	if err != nil {
		log.Debug("LDAP Authan failed for %s, reason: %s", nx, err.Error())
		return "", false
	}

	search := goldap.NewSearchRequest(
		ls.BaseDN,
		goldap.ScopeWholeSubtree, goldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(ls.Filter, name),
		[]string{ls.Attributes},
		nil)
	sr, err := l.Search(search)
	if err != nil {
		log.Debug("LDAP Authen OK but not in filter %s", name)
		return "", false
	}
	log.Debug("LDAP Authen OK: %s", name)
	if len(sr.Entries) > 0 {
		r := sr.Entries[0].GetAttributeValue(ls.Attributes)
		return r, true
	}
	return "", true
}

func ldapDial(ls Ldapsource) (*goldap.Conn, error) {
	if ls.UseSSL {
		return goldap.DialTLS("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port), nil)
	} else {
		return goldap.Dial("tcp", fmt.Sprintf("%s:%d", ls.Host, ls.Port))
	}
}
