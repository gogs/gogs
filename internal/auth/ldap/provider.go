// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ldap

import (
	"fmt"

	"gogs.io/gogs/internal/auth"
)

// Provider contains configuration of an LDAP authentication provider.
type Provider struct {
	directBind bool
	config     *Config
}

// NewProvider creates a new LDAP authentication provider.
func NewProvider(directBind bool, cfg *Config) auth.Provider {
	return &Provider{
		directBind: directBind,
		config:     cfg,
	}
}

// Authenticate queries if login/password is valid against the LDAP directory pool,
// and returns queried information when succeeded.
func (p *Provider) Authenticate(login, password string) (*auth.ExternalAccount, error) {
	username, fn, sn, email, isAdmin, succeed := p.config.searchEntry(login, password, p.directBind)
	if !succeed {
		return nil, auth.ErrBadCredentials{Args: map[string]interface{}{"login": login}}
	}

	if username == "" {
		username = login
	}
	if email == "" {
		email = fmt.Sprintf("%s@localhost", username)
	}

	composeFullName := func(firstname, surname, username string) string {
		switch {
		case firstname == "" && surname == "":
			return username
		case firstname == "":
			return surname
		case surname == "":
			return firstname
		default:
			return firstname + " " + surname
		}
	}

	return &auth.ExternalAccount{
		Login:    login,
		Name:     username,
		FullName: composeFullName(fn, sn, username),
		Email:    email,
		Admin:    isAdmin,
	}, nil
}

func (p *Provider) Config() interface{} {
	return p.config
}

func (p *Provider) HasTLS() bool {
	return p.config.SecurityProtocol > SecurityProtocolUnencrypted
}

func (p *Provider) UseTLS() bool {
	return p.config.SecurityProtocol > SecurityProtocolUnencrypted
}

func (p *Provider) SkipTLSVerify() bool {
	return p.config.SkipVerify
}
