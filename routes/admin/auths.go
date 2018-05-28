// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"fmt"

	"github.com/Unknwon/com"
	"github.com/go-xorm/core"
	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/auth/ldap"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/setting"
)

const (
	AUTHS     = "admin/auth/list"
	AUTH_NEW  = "admin/auth/new"
	AUTH_EDIT = "admin/auth/edit"
)

func Authentications(c *context.Context) {
	c.Title("admin.authentication")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	var err error
	c.Data["Sources"], err = models.LoginSources()
	if err != nil {
		c.ServerError("LoginSources", err)
		return
	}

	c.Data["Total"] = models.CountLoginSources()
	c.Success(AUTHS)
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

func NewAuthSource(c *context.Context) {
	c.Title("admin.auths.new")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["type"] = models.LOGIN_LDAP
	c.Data["CurrentTypeName"] = models.LoginNames[models.LOGIN_LDAP]
	c.Data["CurrentSecurityProtocol"] = models.SecurityProtocolNames[ldap.SECURITY_PROTOCOL_UNENCRYPTED]
	c.Data["smtp_auth"] = "PLAIN"
	c.Data["is_active"] = true
	c.Data["AuthSources"] = authSources
	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = models.SMTPAuths
	c.Success(AUTH_NEW)
}

func parseLDAPConfig(f form.Authentication) *models.LDAPConfig {
	return &models.LDAPConfig{
		Source: &ldap.Source{
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
			GroupEnabled:      f.GroupEnabled,
			GroupDN:           f.GroupDN,
			GroupFilter:       f.GroupFilter,
			GroupMemberUID:    f.GroupMemberUID,
			UserUID:           f.UserUID,
			AdminFilter:       f.AdminFilter,
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

func NewAuthSourcePost(c *context.Context, f form.Authentication) {
	c.Title("admin.auths.new")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["CurrentTypeName"] = models.LoginNames[models.LoginType(f.Type)]
	c.Data["CurrentSecurityProtocol"] = models.SecurityProtocolNames[ldap.SecurityProtocol(f.SecurityProtocol)]
	c.Data["AuthSources"] = authSources
	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = models.SMTPAuths

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
		c.Error(400)
		return
	}
	c.Data["HasTLS"] = hasTLS

	if c.HasError() {
		c.Success(AUTH_NEW)
		return
	}

	if err := models.CreateLoginSource(&models.LoginSource{
		Type:      models.LoginType(f.Type),
		Name:      f.Name,
		IsActived: f.IsActive,
		Cfg:       config,
	}); err != nil {
		if models.IsErrLoginSourceAlreadyExist(err) {
			c.Data["Err_Name"] = true
			c.RenderWithErr(c.Tr("admin.auths.login_source_exist", err.(models.ErrLoginSourceAlreadyExist).Name), AUTH_NEW, f)
		} else {
			c.ServerError("CreateSource", err)
		}
		return
	}

	log.Trace("Authentication created by admin(%s): %s", c.User.Name, f.Name)

	c.Flash.Success(c.Tr("admin.auths.new_success", f.Name))
	c.Redirect(setting.AppSubURL + "/admin/auths")
}

func EditAuthSource(c *context.Context) {
	c.Title("admin.auths.edit")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = models.SMTPAuths

	source, err := models.GetLoginSourceByID(c.ParamsInt64(":authid"))
	if err != nil {
		c.ServerError("GetLoginSourceByID", err)
		return
	}
	c.Data["Source"] = source
	c.Data["HasTLS"] = source.HasTLS()

	c.Success(AUTH_EDIT)
}

func EditAuthSourcePost(c *context.Context, f form.Authentication) {
	c.Title("admin.auths.edit")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["SMTPAuths"] = models.SMTPAuths

	source, err := models.GetLoginSourceByID(c.ParamsInt64(":authid"))
	if err != nil {
		c.ServerError("GetLoginSourceByID", err)
		return
	}
	c.Data["Source"] = source
	c.Data["HasTLS"] = source.HasTLS()

	if c.HasError() {
		c.Success(AUTH_EDIT)
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
		c.Error(400)
		return
	}

	source.Name = f.Name
	source.IsActived = f.IsActive
	source.Cfg = config
	if err := models.UpdateLoginSource(source); err != nil {
		c.ServerError("UpdateLoginSource", err)
		return
	}
	log.Trace("Authentication changed by admin '%s': %d", c.User.Name, source.ID)

	c.Flash.Success(c.Tr("admin.auths.update_success"))
	c.Redirect(setting.AppSubURL + "/admin/auths/" + com.ToStr(f.ID))
}

func DeleteAuthSource(c *context.Context) {
	source, err := models.GetLoginSourceByID(c.ParamsInt64(":authid"))
	if err != nil {
		c.ServerError("GetLoginSourceByID", err)
		return
	}

	if err = models.DeleteSource(source); err != nil {
		if models.IsErrLoginSourceInUse(err) {
			c.Flash.Error(c.Tr("admin.auths.still_in_used"))
		} else {
			c.Flash.Error(fmt.Sprintf("DeleteSource: %v", err))
		}
		c.JSONSuccess(map[string]interface{}{
			"redirect": setting.AppSubURL + "/admin/auths/" + c.Params(":authid"),
		})
		return
	}
	log.Trace("Authentication deleted by admin(%s): %d", c.User.Name, source.ID)

	c.Flash.Success(c.Tr("admin.auths.deletion_success"))
	c.JSONSuccess(map[string]interface{}{
		"redirect": setting.AppSubURL + "/admin/auths",
	})
}
