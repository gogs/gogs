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
	ctx.Data["Sources"], err = models.LoginSources()
	if err != nil {
		ctx.Handle(500, "LoginSources", err)
		return
	}

	ctx.Data["Total"] = models.CountLoginSources()
	ctx.HTML(200, AUTHS)
}

type AuthSource struct {
	Name string
	Type models.LoginType
}

var authSources = []AuthSource{
	{models.LoginNames[models.LDAP], models.LDAP},
	{models.LoginNames[models.DLDAP], models.DLDAP},
	{models.LoginNames[models.SMTP], models.SMTP},
	{models.LoginNames[models.PAM], models.PAM},
}

func NewAuthSource(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.new")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	ctx.Data["type"] = models.LDAP
	ctx.Data["CurTypeName"] = models.LoginNames[models.LDAP]
	ctx.Data["smtp_auth"] = "PLAIN"
	ctx.Data["is_active"] = true
	ctx.Data["AuthSources"] = authSources
	ctx.Data["SMTPAuths"] = models.SMTPAuths
	ctx.HTML(200, AUTH_NEW)
}

func parseLDAPConfig(form auth.AuthenticationForm) *models.LDAPConfig {
	return &models.LDAPConfig{
		Source: &ldap.Source{
			Name:             form.Name,
			Host:             form.Host,
			Port:             form.Port,
			UseSSL:           form.TLS,
			SkipVerify:       form.SkipVerify,
			BindDN:           form.BindDN,
			UserDN:           form.UserDN,
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
}

func parseSMTPConfig(form auth.AuthenticationForm) *models.SMTPConfig {
	return &models.SMTPConfig{
		Auth:           form.SMTPAuth,
		Host:           form.SMTPHost,
		Port:           form.SMTPPort,
		AllowedDomains: form.AllowedDomains,
		TLS:            form.TLS,
		SkipVerify:     form.SkipVerify,
	}
}

func NewAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.new")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	ctx.Data["CurTypeName"] = models.LoginNames[models.LoginType(form.Type)]
	ctx.Data["AuthSources"] = authSources
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	if ctx.HasError() {
		ctx.HTML(200, AUTH_NEW)
		return
	}

	var config core.Conversion
	switch models.LoginType(form.Type) {
	case models.LDAP, models.DLDAP:
		config = parseLDAPConfig(form)
	case models.SMTP:
		config = parseSMTPConfig(form)
	case models.PAM:
		config = &models.PAMConfig{
			ServiceName: form.PAMServiceName,
		}
	default:
		ctx.Error(400)
		return
	}

	if err := models.CreateSource(&models.LoginSource{
		Type:      models.LoginType(form.Type),
		Name:      form.Name,
		IsActived: form.IsActive,
		Cfg:       config,
	}); err != nil {
		ctx.Handle(500, "CreateSource", err)
		return
	}

	log.Trace("Authentication created by admin(%s): %s", ctx.User.Name, form.Name)

	ctx.Flash.Success(ctx.Tr("admin.auths.new_success", form.Name))
	ctx.Redirect(setting.AppSubUrl + "/admin/auths")
}

func EditAuthSource(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.edit")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	ctx.Data["SMTPAuths"] = models.SMTPAuths

	source, err := models.GetLoginSourceByID(ctx.ParamsInt64(":authid"))
	if err != nil {
		ctx.Handle(500, "GetLoginSourceByID", err)
		return
	}
	ctx.Data["Source"] = source
	ctx.HTML(200, AUTH_EDIT)
}

func EditAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.edit")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	ctx.Data["SMTPAuths"] = models.SMTPAuths

	source, err := models.GetLoginSourceByID(ctx.ParamsInt64(":authid"))
	if err != nil {
		ctx.Handle(500, "GetLoginSourceByID", err)
		return
	}
	ctx.Data["Source"] = source

	if ctx.HasError() {
		ctx.HTML(200, AUTH_EDIT)
		return
	}

	var config core.Conversion
	switch models.LoginType(form.Type) {
	case models.LDAP, models.DLDAP:
		config = parseLDAPConfig(form)
	case models.SMTP:
		config = parseSMTPConfig(form)
	case models.PAM:
		config = &models.PAMConfig{
			ServiceName: form.PAMServiceName,
		}
	default:
		ctx.Error(400)
		return
	}

	source.Name = form.Name
	source.IsActived = form.IsActive
	source.Cfg = config
	if err := models.UpdateSource(source); err != nil {
		ctx.Handle(500, "UpdateSource", err)
		return
	}
	log.Trace("Authentication changed by admin(%s): %s", ctx.User.Name, source.ID)

	ctx.Flash.Success(ctx.Tr("admin.auths.update_success"))
	ctx.Redirect(setting.AppSubUrl + "/admin/auths/" + com.ToStr(form.ID))
}

func DeleteAuthSource(ctx *middleware.Context) {
	source, err := models.GetLoginSourceByID(ctx.ParamsInt64(":authid"))
	if err != nil {
		ctx.Handle(500, "GetLoginSourceByID", err)
		return
	}

	if err = models.DeleteSource(source); err != nil {
		switch err {
		case models.ErrAuthenticationUserUsed:
			ctx.Flash.Error("form.still_own_user")
			ctx.Redirect(setting.AppSubUrl + "/admin/auths/" + ctx.Params(":authid"))
		default:
			ctx.Handle(500, "DeleteSource", err)
		}
		return
	}
	log.Trace("Authentication deleted by admin(%s): %d", ctx.User.Name, source.ID)

	ctx.Flash.Success(ctx.Tr("admin.auths.deletion_success"))
	ctx.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubUrl + "/admin/auths",
	})
}
