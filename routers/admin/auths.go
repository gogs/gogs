// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"fmt"

	"github.com/Unknwon/com"
	"github.com/go-xorm/core"
	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/form"
	"github.com/gogits/gogs/modules/setting"
)

const (
	AUTHS     base.TplName = "admin/auth/list"
	AUTH_NEW  base.TplName = "admin/auth/new"
	AUTH_EDIT base.TplName = "admin/auth/edit"
)

func Authentications(ctx *context.Context) {
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

type dropdownItem struct {
	Name string
	Type interface{}
}

var (
	authSources = []dropdownItem{
		{models.LoginNames[models.LOGIN_LDAP], models.LOGIN_LDAP},
		{models.LoginNames[models.LOGIN_DLDAP], models.LOGIN_DLDAP},
		{models.LoginNames[models.LOGIN_SMTP], models.LOGIN_SMTP},
		{models.LoginNames[models.LOGIN_PAM], models.LOGIN_PAM},
	}
	securityProtocols = []dropdownItem{
		{models.SecurityProtocolNames[ldap.SECURITY_PROTOCOL_UNENCRYPTED], ldap.SECURITY_PROTOCOL_UNENCRYPTED},
		{models.SecurityProtocolNames[ldap.SECURITY_PROTOCOL_LDAPS], ldap.SECURITY_PROTOCOL_LDAPS},
		{models.SecurityProtocolNames[ldap.SECURITY_PROTOCOL_START_TLS], ldap.SECURITY_PROTOCOL_START_TLS},
	}
)

func NewAuthSource(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.new")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	ctx.Data["type"] = models.LOGIN_LDAP
	ctx.Data["CurrentTypeName"] = models.LoginNames[models.LOGIN_LDAP]
	ctx.Data["CurrentSecurityProtocol"] = models.SecurityProtocolNames[ldap.SECURITY_PROTOCOL_UNENCRYPTED]
	ctx.Data["smtp_auth"] = "PLAIN"
	ctx.Data["is_active"] = true
	ctx.Data["AuthSources"] = authSources
	ctx.Data["SecurityProtocols"] = securityProtocols
	ctx.Data["SMTPAuths"] = models.SMTPAuths
	ctx.HTML(200, AUTH_NEW)
}

func parseLDAPConfig(f form.Authentication) *models.LDAPConfig {
	return &models.LDAPConfig{
		Source: &ldap.Source{
			Name:              f.Name,
			Host:              f.Host,
			Port:              f.Port,
			SecurityProtocol:  ldap.SecurityProtocol(f.SecurityProtocol),
			SkipVerify:        f.SkipVerify,
			BindDN:            f.BindDN,
			UserDN:            f.UserDN,
			BindPassword:      f.BindPassword,
			UserBase:          f.UserBase,
			AttributeUsername: f.AttributeUsername,
			AttributeName:     f.AttributeName,
			AttributeSurname:  f.AttributeSurname,
			AttributeMail:     f.AttributeMail,
			AttributesInBind:  f.AttributesInBind,
			Filter:            f.Filter,
			AdminFilter:       f.AdminFilter,
			Enabled:           true,
		},
	}
}

func parseSMTPConfig(f form.Authentication) *models.SMTPConfig {
	return &models.SMTPConfig{
		Auth:           f.SMTPAuth,
		Host:           f.SMTPHost,
		Port:           f.SMTPPort,
		AllowedDomains: f.AllowedDomains,
		TLS:            f.TLS,
		SkipVerify:     f.SkipVerify,
	}
}

func NewAuthSourcePost(ctx *context.Context, f form.Authentication) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.new")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	ctx.Data["CurrentTypeName"] = models.LoginNames[models.LoginType(f.Type)]
	ctx.Data["CurrentSecurityProtocol"] = models.SecurityProtocolNames[ldap.SecurityProtocol(f.SecurityProtocol)]
	ctx.Data["AuthSources"] = authSources
	ctx.Data["SecurityProtocols"] = securityProtocols
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	hasTLS := false
	var config core.Conversion
	switch models.LoginType(f.Type) {
	case models.LOGIN_LDAP, models.LOGIN_DLDAP:
		config = parseLDAPConfig(f)
		hasTLS = ldap.SecurityProtocol(f.SecurityProtocol) > ldap.SECURITY_PROTOCOL_UNENCRYPTED
	case models.LOGIN_SMTP:
		config = parseSMTPConfig(f)
		hasTLS = true
	case models.LOGIN_PAM:
		config = &models.PAMConfig{
			ServiceName: f.PAMServiceName,
		}
	default:
		ctx.Error(400)
		return
	}
	ctx.Data["HasTLS"] = hasTLS

	if ctx.HasError() {
		ctx.HTML(200, AUTH_NEW)
		return
	}

	if err := models.CreateLoginSource(&models.LoginSource{
		Type:      models.LoginType(f.Type),
		Name:      f.Name,
		IsActived: f.IsActive,
		Cfg:       config,
	}); err != nil {
		if models.IsErrLoginSourceAlreadyExist(err) {
			ctx.Data["Err_Name"] = true
			ctx.RenderWithErr(ctx.Tr("admin.auths.login_source_exist", err.(models.ErrLoginSourceAlreadyExist).Name), AUTH_NEW, f)
		} else {
			ctx.Handle(500, "CreateSource", err)
		}
		return
	}

	log.Trace("Authentication created by admin(%s): %s", ctx.User.Name, f.Name)

	ctx.Flash.Success(ctx.Tr("admin.auths.new_success", f.Name))
	ctx.Redirect(setting.AppSubUrl + "/admin/auths")
}

func EditAuthSource(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.auths.edit")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminAuthentications"] = true

	ctx.Data["SecurityProtocols"] = securityProtocols
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	source, err := models.GetLoginSourceByID(ctx.ParamsInt64(":authid"))
	if err != nil {
		ctx.Handle(500, "GetLoginSourceByID", err)
		return
	}
	ctx.Data["Source"] = source
	ctx.Data["HasTLS"] = source.HasTLS()

	ctx.HTML(200, AUTH_EDIT)
}

func EditAuthSourcePost(ctx *context.Context, f form.Authentication) {
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
	ctx.Data["HasTLS"] = source.HasTLS()

	if ctx.HasError() {
		ctx.HTML(200, AUTH_EDIT)
		return
	}

	var config core.Conversion
	switch models.LoginType(f.Type) {
	case models.LOGIN_LDAP, models.LOGIN_DLDAP:
		config = parseLDAPConfig(f)
	case models.LOGIN_SMTP:
		config = parseSMTPConfig(f)
	case models.LOGIN_PAM:
		config = &models.PAMConfig{
			ServiceName: f.PAMServiceName,
		}
	default:
		ctx.Error(400)
		return
	}

	source.Name = f.Name
	source.IsActived = f.IsActive
	source.Cfg = config
	if err := models.UpdateSource(source); err != nil {
		ctx.Handle(500, "UpdateSource", err)
		return
	}
	log.Trace("Authentication changed by admin(%s): %d", ctx.User.Name, source.ID)

	ctx.Flash.Success(ctx.Tr("admin.auths.update_success"))
	ctx.Redirect(setting.AppSubUrl + "/admin/auths/" + com.ToStr(f.ID))
}

func DeleteAuthSource(ctx *context.Context) {
	source, err := models.GetLoginSourceByID(ctx.ParamsInt64(":authid"))
	if err != nil {
		ctx.Handle(500, "GetLoginSourceByID", err)
		return
	}

	if err = models.DeleteSource(source); err != nil {
		if models.IsErrLoginSourceInUse(err) {
			ctx.Flash.Error(ctx.Tr("admin.auths.still_in_used"))
		} else {
			ctx.Flash.Error(fmt.Sprintf("DeleteSource: %v", err))
		}
		ctx.JSON(200, map[string]interface{}{
			"redirect": setting.AppSubUrl + "/admin/auths/" + ctx.Params(":authid"),
		})
		return
	}
	log.Trace("Authentication deleted by admin(%s): %d", ctx.User.Name, source.ID)

	ctx.Flash.Success(ctx.Tr("admin.auths.deletion_success"))
	ctx.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubUrl + "/admin/auths",
	})
}
