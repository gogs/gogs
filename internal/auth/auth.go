// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"

	"gogs.io/gogs/internal/errutil"
)

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

var _ errutil.NotFound = (*ErrBadCredentials)(nil)

type ErrBadCredentials struct {
	Args errutil.Args
}

func IsErrBadCredentials(err error) bool {
	_, ok := err.(ErrBadCredentials)
	return ok
}

func (err ErrBadCredentials) Error() string {
	return fmt.Sprintf("bad credentials: %v", err.Args)
}

func (ErrBadCredentials) NotFound() bool {
	return true
}

// ExternalAccount contains queried information returned by an authenticate provider
// for an external account.
type ExternalAccount struct {
	// REQUIRED: The login to be used for authenticating against the provider.
	Login string
	// REQUIRED: The username of the account.
	Name string
	// The full name of the account.
	FullName string
	// The email address of the account.
	Email string
	// The location of the account.
	Location string
	// The website of the account.
	Website string
	// Whether the user should be prompted as a site admin.
	Admin bool
}

// Provider defines an authenticate provider which provides ability to authentication against
// an external identity provider and query external account information.
type Provider interface {
	// Authenticate performs authentication against an external identity provider
	// using given credentials and returns queried information of the external account.
	Authenticate(login, password string) (*ExternalAccount, error)

	// Config returns the underlying configuration of the authenticate provider.
	Config() interface{}
	// HasTLS returns true if the authenticate provider supports TLS.
	HasTLS() bool
	// UseTLS returns true if the authenticate provider is configured to use TLS.
	UseTLS() bool
	// SkipTLSVerify returns true if the authenticate provider is configured to skip TLS verify.
	SkipTLSVerify() bool
}
