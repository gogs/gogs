// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.gogs file.

package form

import (
	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"
)

type Authentication struct {
	ID                int64
	Type              int    `binding:"Range(2,6)"`
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
}

func (f *Authentication) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
