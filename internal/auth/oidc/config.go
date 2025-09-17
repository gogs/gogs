// Copyright 2024 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package oidc

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config contains configuration of an OIDC authentication provider.
type Config struct {
	IssuerURL    string `json:"issuer_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
	Scopes       string `json:"scopes"`
	AutoRegister bool   `json:"auto_register"`
	SkipVerify   bool   `json:"skip_verify"`
}

// newProvider creates the OIDC provider and OAuth2 config
func (cfg *Config) newProvider(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OIDC provider: %v", err)
	}

	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if cfg.Scopes != "" {
		// TODO: Parse additional scopes from config
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL, // This will be dynamically set in the handler
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	return provider, oauth2Config, nil
}