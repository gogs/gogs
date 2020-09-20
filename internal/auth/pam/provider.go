// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package pam

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
	err := p.config.doAuth(login, password)
	if err != nil {
		if strings.Contains(err.Error(), "Authentication failure") {
			return nil, auth.ErrBadCredentials{Args: map[string]interface{}{"login": login}}
		}
		return nil, err
	}

	return &auth.ExternalAccount{
		Login: login,
		Name:  login,
	}, nil
}

func (p *Provider) Config() interface{} {
	return p.config
}

func (p *Provider) HasTLS() bool {
	return false
}

func (p *Provider) UseTLS() bool {
	return false
}

func (p *Provider) SkipTLSVerify() bool {
	return false
}
