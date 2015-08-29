// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/binding"
)

type AuthenticationForm struct {
	ID                int64 `form:"id"`
	Type              int
	Name              string `binding:"Required;MaxSize(50)"`
	Host              string
	Port              int
	UseSSL            bool   `form:"use_ssl"`
	BindDN            string `form:"bind_dn"`
	BindPassword      string
	UserBase          string
	AttributeName     string
	AttributeSurname  string
	AttributeMail     string
	Filter            string
	AdminFilter       string
	IsActived         bool
	SMTPAuth          string `form:"smtp_auth"`
	SMTPHost          string `form:"smtp_host"`
	SMTPPort          int    `form:"smtp_port"`
	TLS               bool   `form:"tls"`
	SkipVerify        bool
	AllowAutoRegister bool `form:"allowautoregister"`
	PAMServiceName    string
}

func (f *AuthenticationForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
