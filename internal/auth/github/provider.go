// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package github

import (
	"strings"

	"gogs.io/gogs/internal/auth"
)

// Provider contains configuration of a PAM authentication provider.
type Provider struct {
	config *Config
}

// NewProvider creates a new PAM authentication provider.
func NewProvider(cfg *Config) auth.Provider {
	return &Provider{
		config: cfg,
	}
}

func (p *Provider) Authenticate(login, password string) (*auth.ExternalAccount, error) {
	fullname, email, website, location, err := p.config.doAuth(login, password)
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			return nil, auth.ErrBadCredentials{Args: map[string]any{"login": login}}
		}
		return nil, err
	}
	return &auth.ExternalAccount{
		Login:    login,
		Name:     login,
		FullName: fullname,
		Email:    email,
		Location: location,
		Website:  website,
	}, nil
}

func (p *Provider) Config() any {
	return p.config
}

func (*Provider) HasTLS() bool {
	return true
}

func (*Provider) UseTLS() bool {
	return true
}

func (p *Provider) SkipTLSVerify() bool {
	return p.config.SkipVerify
}
