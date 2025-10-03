// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package form

import (
	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"
)

type Authentication struct {
	ID                int64
	Type              int    `binding:"Range(2,7)"`
	Name              string `binding:"Required;MaxSize(30)"`
	Host              string
	Port              int
	BindDN            string
	BindPassword      string
	UserBase          string
	UserDN            string
	AttributeUsername string
	AttributeName     string
	AttributeSurname  string
	AttributeMail     string
	AttributesInBind  bool
	Filter            string
	AdminFilter       string
	GroupEnabled      bool
	GroupDN           string
	GroupFilter       string
	GroupMemberUID    string
	UserUID           string
	IsActive          bool
	IsDefault         bool
	SMTPAuth          string
	SMTPHost          string
	SMTPPort          int
	AllowedDomains    string
	SecurityProtocol  int `binding:"Range(0,2)"`
	TLS               bool
	SkipVerify        bool
	PAMServiceName    string
	GitHubAPIEndpoint string `form:"github_api_endpoint" binding:"Url"`
	// OIDC fields
	OIDCIssuerURL       string `form:"oidc_issuer_url" binding:"Required;Url"`
	OIDCClientID        string `form:"oidc_client_id" binding:"Required"`
	OIDCClientSecret    string `form:"oidc_client_secret" binding:"Required"`
	OIDCScopes          string `form:"oidc_scopes"`
	OIDCAutoRegister    bool   `form:"oidc_auto_register"`
	OIDCAdminGroup      string `form:"oidc_admin_group"`
	OIDCButtonLogoURL   string `form:"oidc_button_logo_url"`
	OIDCButtonBgColor   string `form:"oidc_button_bg_color"`
	OIDCButtonTextColor string `form:"oidc_button_text_color"`
}

func (f *Authentication) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
