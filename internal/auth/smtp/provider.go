// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package smtp

import (
	"net/smtp"
	"net/textproto"
	"strings"

	"github.com/pkg/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
)

// Provider contains configuration of an SMTP authentication provider.
type Provider struct {
	config *Config
}

// NewProvider creates a new SMTP authentication provider.
func NewProvider(cfg *Config) auth.Provider {
	return &Provider{
		config: cfg,
	}
}

// Authenticate queries if login/password is valid against the SMTP server,
// and returns queried information when succeeded.
func (p *Provider) Authenticate(login, password string) (*auth.ExternalAccount, error) {
	// Verify allowed domains
	if p.config.AllowedDomains != "" {
		fields := strings.SplitN(login, "@", 3)
		if len(fields) != 2 {
			return nil, auth.ErrBadCredentials{Args: map[string]interface{}{"login": login}}
		}
		domain := fields[1]

		isAllowed := false
		for _, allowed := range strings.Split(p.config.AllowedDomains, ",") {
			if domain == allowed {
				isAllowed = true
				break
			}
		}

		if !isAllowed {
			return nil, auth.ErrBadCredentials{Args: map[string]interface{}{"login": login}}
		}
	}

	var smtpAuth smtp.Auth
	switch p.config.Auth {
	case Plain:
		smtpAuth = smtp.PlainAuth("", login, password, p.config.Host)
	case Login:
		smtpAuth = &smtpLoginAuth{login, password}
	default:
		return nil, errors.Errorf("unsupported SMTP authentication type %q", p.config.Auth)
	}

	if err := p.config.doAuth(smtpAuth); err != nil {
		log.Trace("SMTP: Authentication failed: %v", err)

		// Check standard error format first, then fallback to the worse case.
		tperr, ok := err.(*textproto.Error)
		if (ok && tperr.Code == 535) ||
			strings.Contains(err.Error(), "Username and Password not accepted") {
			return nil, auth.ErrBadCredentials{Args: map[string]interface{}{"login": login}}
		}
		return nil, err
	}

	username := login

	// NOTE: It is not required to have "@" in `login` for a successful SMTP authentication.
	idx := strings.Index(login, "@")
	if idx > -1 {
		username = login[:idx]
	}

	return &auth.ExternalAccount{
		Login: login,
		Name:  username,
		Email: login,
	}, nil
}

func (p *Provider) Config() interface{} {
	return p.config
}

func (p *Provider) HasTLS() bool {
	return true
}

func (p *Provider) UseTLS() bool {
	return p.config.TLS
}

func (p *Provider) SkipTLSVerify() bool {
	return p.config.SkipVerify
}

const (
	Plain = "PLAIN"
	Login = "LOGIN"
)

var AuthTypes = []string{Plain, Login}

type smtpLoginAuth struct {
	username, password string
}

func (auth *smtpLoginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(auth.username), nil
}

func (auth *smtpLoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(auth.username), nil
		case "Password:":
			return []byte(auth.password), nil
		}
	}
	return nil, nil
}
