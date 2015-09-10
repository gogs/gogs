// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/binding"
)

type AuthenticationForm struct {
	ID                int64
	Type              int    `binding:"Range(2,5)"`
	Name              string `binding:"Required;MaxSize(30)"`
	Host              string
	Port              int
	UseSSL            bool
	BindDN            string
	BindPassword      string
	UserBase          string
	UserDN            string `form:"user_dn"`
	AttributeName     string
	AttributeSurname  string
	AttributeMail     string
	Filter            string
	AdminFilter       string
	IsActive          bool
	SMTPAuth          string
	SMTPHost          string
	SMTPPort          int
	TLS               bool
	SkipVerify        bool
	AllowAutoRegister bool
	PAMServiceName    string `form:"pam_service_name"`
}

func (f *AuthenticationForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
