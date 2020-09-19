// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// FIXME: Put this file into its own package and separate into different files based on login sources.
package db

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"net/textproto"
	"strings"

	"github.com/unknwon/com"

	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/db/errors"
)

// ***********************
// ----- SMTP config -----
// ***********************

type SMTPConfig struct {
	Auth           string
	Host           string
	Port           int
	AllowedDomains string
	TLS            bool `ini:"tls"`
	SkipVerify     bool
}

type smtpLoginAuth struct {
	username, password string
}

func (auth *smtpLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
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

const (
	SMTPPlain = "PLAIN"
	SMTPLogin = "LOGIN"
)

var SMTPAuths = []string{SMTPPlain, SMTPLogin}

func SMTPAuth(a smtp.Auth, cfg *SMTPConfig) error {
	c, err := smtp.Dial(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	if err != nil {
		return err
	}
	defer c.Close()

	if err = c.Hello("gogs"); err != nil {
		return err
	}

	if cfg.TLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err = c.StartTLS(&tls.Config{
				InsecureSkipVerify: cfg.SkipVerify,
				ServerName:         cfg.Host,
			}); err != nil {
				return err
			}
		} else {
			return errors.New("SMTP server unsupports TLS")
		}
	}

	if ok, _ := c.Extension("AUTH"); ok {
		if err = c.Auth(a); err != nil {
			return err
		}
		return nil
	}
	return errors.New("Unsupported SMTP authentication method")
}

// LoginViaSMTP queries if login/password is valid against the SMTP,
// and create a local user if success when enabled.
func LoginViaSMTP(login, password string, sourceID int64, cfg *SMTPConfig, autoRegister bool) (*User, error) {
	// Verify allowed domains.
	if len(cfg.AllowedDomains) > 0 {
		idx := strings.Index(login, "@")
		if idx == -1 {
			return nil, ErrUserNotExist{args: map[string]interface{}{"login": login}}
		} else if !com.IsSliceContainsStr(strings.Split(cfg.AllowedDomains, ","), login[idx+1:]) {
			return nil, ErrUserNotExist{args: map[string]interface{}{"login": login}}
		}
	}

	var auth smtp.Auth
	if cfg.Auth == SMTPPlain {
		auth = smtp.PlainAuth("", login, password, cfg.Host)
	} else if cfg.Auth == SMTPLogin {
		auth = &smtpLoginAuth{login, password}
	} else {
		return nil, errors.New("Unsupported SMTP authentication type")
	}

	if err := SMTPAuth(auth, cfg); err != nil {
		// Check standard error format first,
		// then fallback to worse case.
		tperr, ok := err.(*textproto.Error)
		if (ok && tperr.Code == 535) ||
			strings.Contains(err.Error(), "Username and Password not accepted") {
			return nil, ErrUserNotExist{args: map[string]interface{}{"login": login}}
		}
		return nil, err
	}

	if !autoRegister {
		return nil, nil
	}

	username := login
	idx := strings.Index(login, "@")
	if idx > -1 {
		username = login[:idx]
	}

	user := &User{
		LowerName:   strings.ToLower(username),
		Name:        strings.ToLower(username),
		Email:       login,
		Passwd:      password,
		LoginSource: sourceID,
		LoginName:   login,
		IsActive:    true,
	}
	return user, CreateUser(user)
}

// **********************
// ----- PAM config -----
// **********************

type PAMConfig struct {
	// The name of the PAM service, e.g. system-auth.
	ServiceName string
}

// LoginViaPAM queries if login/password is valid against the PAM,
// and create a local user if success when enabled.
func LoginViaPAM(login, password string, sourceID int64, cfg *PAMConfig, autoRegister bool) (*User, error) {
	if err := pam.PAMAuth(cfg.ServiceName, login, password); err != nil {
		if strings.Contains(err.Error(), "Authentication failure") {
			return nil, ErrUserNotExist{args: map[string]interface{}{"login": login}}
		}
		return nil, err
	}

	if !autoRegister {
		return nil, nil
	}

	user := &User{
		LowerName:   strings.ToLower(login),
		Name:        login,
		Email:       login,
		Passwd:      password,
		LoginSource: sourceID,
		LoginName:   login,
		IsActive:    true,
	}
	return user, CreateUser(user)
}

// *************************
// ----- GitHub config -----
// *************************

type GitHubConfig struct {
	// the GitHub service endpoint, e.g. https://api.github.com/.
	APIEndpoint string
}

func LoginViaGitHub(login, password string, sourceID int64, cfg *GitHubConfig, autoRegister bool) (*User, error) {
	fullname, email, url, location, err := github.Authenticate(cfg.APIEndpoint, login, password)
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			return nil, ErrUserNotExist{args: map[string]interface{}{"login": login}}
		}
		return nil, err
	}

	if !autoRegister {
		return nil, nil
	}
	user := &User{
		LowerName:   strings.ToLower(login),
		Name:        login,
		FullName:    fullname,
		Email:       email,
		Website:     url,
		Passwd:      password,
		LoginSource: sourceID,
		LoginName:   login,
		IsActive:    true,
		Location:    location,
	}
	return user, CreateUser(user)
}

// TODO: Delete me
func authenticateViaLoginSource(source *LoginSource, login, password string, autoRegister bool) (*User, error) {
	if !source.IsActived {
		return nil, errors.LoginSourceNotActivated{SourceID: source.ID}
	}

	extAccount, err := source.Provider.Authenticate(login, password)
	if err != nil {
		return nil, err
	}

	_ = extAccount // TODO
	return nil, nil
}
