// Copyright 2024 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package oidc

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"gogs.io/gogs/internal/auth"
)

// Provider contains configuration of an OIDC authentication provider.
type Provider struct {
	config *Config
}

// NewProvider creates a new OIDC authentication provider.
func NewProvider(cfg *Config) auth.Provider {
	return &Provider{
		config: cfg,
	}
}

// Authenticate performs authentication against OIDC provider.
// For OIDC, this method is not used for direct login but for validation.
func (p *Provider) Authenticate(login, password string) (*auth.ExternalAccount, error) {
	// OIDC authentication is handled via OAuth2 flow, not username/password
	return nil, fmt.Errorf("OIDC authentication requires OAuth2 flow")
}

// GetOAuth2Config returns the OIDC provider and OAuth2 config for login flow
func (p *Provider) GetOAuth2Config(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	return p.config.newProvider(ctx)
}

// AuthenticateUser validates an OIDC token and returns user information.
func (p *Provider) AuthenticateUser(ctx context.Context, code string) (*auth.ExternalAccount, error) {
	provider, oauth2Config, err := p.config.newProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %v", err)
	}

	// Exchange code for token
	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %v", err)
	}

	// Verify the token
	verifier := provider.Verifier(&oidc.Config{ClientID: p.config.ClientID})
	idToken, err := verifier.Verify(ctx, token.Extra("id_token").(string))
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %v", err)
	}

	// Extract claims
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		PreferredUsername string `json:"preferred_username"`
		Subject       string `json:"sub"`
	}

	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse ID token claims: %v", err)
	}

	// Build external account
	login := claims.PreferredUsername
	if login == "" {
		login = claims.Email
	}
	if login == "" {
		login = claims.Subject
	}

	fullName := claims.Name
	if fullName == "" && claims.GivenName != "" && claims.FamilyName != "" {
		fullName = claims.GivenName + " " + claims.FamilyName
	}

	return &auth.ExternalAccount{
		Login:    login,
		Name:     login,
		FullName: fullName,
		Email:    claims.Email,
	}, nil
}

func (p *Provider) Config() any {
	return p.config
}

func (p *Provider) HasTLS() bool {
	return true
}

func (p *Provider) UseTLS() bool {
	return true
}

func (p *Provider) SkipTLSVerify() bool {
	return p.config.SkipVerify
}