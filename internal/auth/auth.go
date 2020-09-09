// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

type LoginType int

// Note: New type must append to the end of list to maintain backward compatibility.
const (
	LoginNotype LoginType = iota
	LoginPlain            // 1
	LoginLDAP             // 2
	LoginSMTP             // 3
	LoginPAM              // 4
	LoginDLDAP            // 5
	LoginGitHub           // 6
)

// LoginNames returns the human-readable name for given authentication type.
func LoginNames(loginType LoginType) string {
	return map[LoginType]string{
		LoginLDAP:   "LDAP (via BindDN)",
		LoginDLDAP:  "LDAP (simple auth)", // Via direct bind
		LoginSMTP:   "SMTP",
		LoginPAM:    "PAM",
		LoginGitHub: "GitHub",
	}[loginType]
}
