// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// FIXME: Put this file into its own package and separate into different files based on login sources.
package db

import (
	"strings"

	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/pam"
)

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
