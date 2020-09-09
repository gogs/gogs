// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

type Type int

// Note: New type must append to the end of list to maintain backward compatibility.
const (
	None   Type = iota
	Plain       // 1
	LDAP        // 2
	SMTP        // 3
	PAM         // 4
	DLDAP       // 5
	GitHub      // 6
)

// Name returns the human-readable name for given authentication type.
func Name(typ Type) string {
	return map[Type]string{
		LDAP:   "LDAP (via BindDN)",
		DLDAP:  "LDAP (simple auth)", // Via direct bind
		SMTP:   "SMTP",
		PAM:    "PAM",
		GitHub: "GitHub",
	}[typ]
}
