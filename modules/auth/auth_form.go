// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/binding"
)

type AuthenticationForm struct {
	Id                int64  `form:"id"`
	Type              int    `form:"type"`
	AuthName          string `form:"name" binding:"Required;MaxSize(50)"`
	Domain            string `form:"domain"`
	Host              string `form:"host"`
	Port              int    `form:"port"`
	UseSSL            bool   `form:"usessl"`
	BaseDN            string `form:"base_dn"`
	AttributeUsername string `form:"attribute_username"`
	AttributeName     string `form:"attribute_name"`
	AttributeSurname  string `form:"attribute_surname"`
	AttributeMail     string `form:"attribute_mail"`
	Filter            string `form:"filter"`
	MsAdSA            string `form:"ms_ad_sa"`
	IsActived         bool   `form:"is_actived"`
	SmtpAuth          string `form:"smtpauth"`
	SmtpHost          string `form:"smtphost"`
	SmtpPort          int    `form:"smtpport"`
	Tls               bool   `form:"tls"`
	AllowAutoRegister bool   `form:"allowautoregister"`
}

func (f *AuthenticationForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
