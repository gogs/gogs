// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

// Config contains configuration for GitHub authentication.
//
// ⚠️ WARNING: Change to the field name must preserve the INI key name for backward compatibility.
type Config struct {
	// the GitHub service endpoint, e.g. https://api.github.com/.
	APIEndpoint string
	SkipVerify  bool
}

func (c *Config) doAuth(login, password string) (fullname, email, location, website string, err error) {
	tp := github.BasicAuthTransport{
		Username: strings.TrimSpace(login),
		Password: strings.TrimSpace(password),
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipVerify},
		},
	}
	client, err := github.NewEnterpriseClient(c.APIEndpoint, c.APIEndpoint, tp.Client())
	if err != nil {
		return "", "", "", "", errors.Wrap(err, "create new client")
	}
	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return "", "", "", "", errors.Wrap(err, "get user info")
	}

	if user.Name != nil {
		fullname = *user.Name
	}
	if user.Email != nil {
		email = *user.Email
	} else {
		email = login + "+github@local"
	}
	if user.Location != nil {
		location = strings.ToUpper(*user.Location)
	}
	if user.HTMLURL != nil {
		website = strings.ToLower(*user.HTMLURL)
	}
	return fullname, email, location, website, nil
}
