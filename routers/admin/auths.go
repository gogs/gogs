// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/com"
	"github.com/go-xorm/core"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	AUTHS     base.TplName = "admin/auth/list"
	AUTH_NEW  base.TplName = "admin/auth/new"
	AUTH_EDIT base.TplName = "admin/auth/edit"
)

func Authentications(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.authentication")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	var err error
	ctx.Data["Sources"], err = models.GetAuths()
	if err != nil {
		ctx.Handle(500, "GetAuths", err)
		return
	}
	ctx.HTML(200, AUTHS)
}

func NewAuthSource(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.new")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths
	ctx.HTML(200, AUTH_NEW)
}

func NewAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.new")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	if ctx.HasError() {
		ctx.HTML(200, AUTH_NEW)
		return
	}

	var u core.Conversion
	switch models.LoginType(form.Type) {
	case models.LDAP:
		u = &models.LDAPConfig{
			Ldapsource: ldap.Ldapsource{
				Name:             form.Name,
				Host:             form.Host,
				Port:             form.Port,
				UseSSL:           form.UseSSL,
				BindDN:           form.BindDN,
				BindPassword:     form.BindPassword,
				UserBase:         form.UserBase,
				Filter:           form.Filter,
				AdminFilter:      form.AdminFilter,
				AttributeName:    form.AttributeName,
				AttributeSurname: form.AttributeSurname,
				AttributeMail:    form.AttributeMail,
				Enabled:          true,
			},
		}
	case models.SMTP:
		u = &models.SMTPConfig{
			Auth:       form.SMTPAuth,
			Host:       form.SMTPHost,
			Port:       form.SMTPPort,
			TLS:        form.TLS,
			SkipVerify: form.SkipVerify,
		}
	case models.PAM:
		u = &models.PAMConfig{
			ServiceName: form.PAMServiceName,
		}
	default:
		ctx.Error(400)
		return
	}

	var source = &models.LoginSource{
		Type:              models.LoginType(form.Type),
		Name:              form.Name,
		IsActived:         true,
		AllowAutoRegister: form.AllowAutoRegister,
		Cfg:               u,
	}

	if err := models.CreateSource(source); err != nil {
		ctx.Handle(500, "CreateSource", err)
		return
	}

	log.Trace("Authentication created by admin(%s): %s", ctx.User.Name, form.Name)
	ctx.Redirect(setting.AppSubUrl + "/admin/auths")
}

func EditAuthSource(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.edit")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	id := com.StrTo(ctx.Params(":authid")).MustInt64()
	if id == 0 {
		ctx.Handle(404, "EditAuthSource", nil)
		return
	}
	u, err := models.GetLoginSourceByID(id)
	if err != nil {
		ctx.Handle(500, "GetLoginSourceById", err)
		return
	}
	ctx.Data["Source"] = u
	ctx.HTML(200, AUTH_EDIT)
}

func EditAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.edit")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true
	ctx.Data["PageIsAuths"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	if ctx.HasError() {
		ctx.HTML(200, AUTH_EDIT)
		return
	}

	var config core.Conversion
	switch models.LoginType(form.Type) {
	case models.LDAP:
		config = &models.LDAPConfig{
			Ldapsource: ldap.Ldapsource{
				Name:             form.Name,
				Host:             form.Host,
				Port:             form.Port,
				UseSSL:           form.UseSSL,
				BindDN:           form.BindDN,
				BindPassword:     form.BindPassword,
				UserBase:         form.UserBase,
				AttributeName:    form.AttributeName,
				AttributeSurname: form.AttributeSurname,
				AttributeMail:    form.AttributeMail,
				Filter:           form.Filter,
				AdminFilter:      form.AdminFilter,
				Enabled:          true,
			},
		}
	case models.SMTP:
		config = &models.SMTPConfig{
			Auth:       form.SMTPAuth,
			Host:       form.SMTPHost,
			Port:       form.SMTPPort,
			TLS:        form.TLS,
			SkipVerify: form.SkipVerify,
		}
	case models.PAM:
		config = &models.PAMConfig{
			ServiceName: form.PAMServiceName,
		}
	default:
		ctx.Error(400)
		return
	}

	u := models.LoginSource{
		ID:                form.ID,
		Name:              form.Name,
		IsActived:         form.IsActived,
		Type:              models.LoginType(form.Type),
		AllowAutoRegister: form.AllowAutoRegister,
		Cfg:               config,
	}

	if err := models.UpdateSource(&u); err != nil {
		ctx.Handle(500, "UpdateSource", err)
		return
	}

	log.Trace("Authentication changed by admin(%s): %s", ctx.User.Name, form.Name)
	ctx.Flash.Success(ctx.Tr("admin.auths.update_success"))
	ctx.Redirect(setting.AppSubUrl + "/admin/auths/" + ctx.Params(":authid"))
}

func DeleteAuthSource(ctx *middleware.Context) {
	id := com.StrTo(ctx.Params(":authid")).MustInt64()
	if id == 0 {
		ctx.Handle(404, "DeleteAuthSource", nil)
		return
	}

	a, err := models.GetLoginSourceByID(id)
	if err != nil {
		ctx.Handle(500, "GetLoginSourceById", err)
		return
	}

	if err = models.DelLoginSource(a); err != nil {
		switch err {
		case models.ErrAuthenticationUserUsed:
			ctx.Flash.Error("form.still_own_user")
			ctx.Redirect(setting.AppSubUrl + "/admin/auths/" + ctx.Params(":authid"))
		default:
			ctx.Handle(500, "DelLoginSource", err)
		}
		return
	}
	log.Trace("Authentication deleted by admin(%s): %s", ctx.User.Name, a.Name)
	ctx.Redirect(setting.AppSubUrl + "/admin/auths")
}
