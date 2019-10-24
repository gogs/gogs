// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
)

func Authenticate(apiEndpoint, login, passwd string) (name string, email string, website string, location string, _ error) {
	tp := github.BasicAuthTransport{
		Username: strings.TrimSpace(login),
		Password: strings.TrimSpace(passwd),
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	client, err := github.NewEnterpriseClient(apiEndpoint, apiEndpoint, tp.Client())
	if err != nil {
		return "", "", "", "", fmt.Errorf("create new client: %v", err)
	}
	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return "", "", "", "", fmt.Errorf("get user info: %v", err)
	}

	if user.Name != nil {
		name = *user.Name
	}
	if user.Email != nil {
		email = *user.Email
	} else {
		email = login + "+github@local"
	}
	if user.HTMLURL != nil {
		website = strings.ToLower(*user.HTMLURL)
	}
	if user.Location != nil {
		location = strings.ToUpper(*user.Location)
	}

	return name, email, website, location, nil
}
