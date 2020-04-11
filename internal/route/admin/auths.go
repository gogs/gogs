// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/form"
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
	c.Data["Sources"], err = db.LoginSources.List(db.ListLoginSourceOpts{})
	if err != nil {
		c.Error(err, "list login sources")
		return
	}

	c.Data["Total"] = db.LoginSources.Count()
	c.Success(AUTHS)
}

type dropdownItem struct {
	Name string
	Type interface{}
}

var (
	authSources = []dropdownItem{
		{db.LoginNames[db.LoginLDAP], db.LoginLDAP},
		{db.LoginNames[db.LoginDLDAP], db.LoginDLDAP},
		{db.LoginNames[db.LoginSMTP], db.LoginSMTP},
		{db.LoginNames[db.LoginPAM], db.LoginPAM},
		{db.LoginNames[db.LoginGitHub], db.LoginGitHub},
	}
	securityProtocols = []dropdownItem{
		{db.SecurityProtocolNames[ldap.SecurityProtocolUnencrypted], ldap.SecurityProtocolUnencrypted},
		{db.SecurityProtocolNames[ldap.SecurityProtocolLDAPS], ldap.SecurityProtocolLDAPS},
		{db.SecurityProtocolNames[ldap.SecurityProtocolStartTLS], ldap.SecurityProtocolStartTLS},
	}
)

func NewAuthSource(c *context.Context) {
	c.Title("admin.auths.new")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["type"] = db.LoginLDAP
	c.Data["CurrentTypeName"] = db.LoginNames[db.LoginLDAP]
	c.Data["CurrentSecurityProtocol"] = db.SecurityProtocolNames[ldap.SecurityProtocolUnencrypted]
	c.Data["smtp_auth"] = "PLAIN"
	c.Data["is_active"] = true
	c.Data["is_default"] = true
	c.Data["AuthSources"] = authSources
	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = db.SMTPAuths
	c.Success(AUTH_NEW)
}

func parseLDAPConfig(f form.Authentication) *db.LDAPConfig {
	return &db.LDAPConfig{
		Source: ldap.Source{
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

func parseSMTPConfig(f form.Authentication) *db.SMTPConfig {
	return &db.SMTPConfig{
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

	c.Data["CurrentTypeName"] = db.LoginNames[db.LoginType(f.Type)]
	c.Data["CurrentSecurityProtocol"] = db.SecurityProtocolNames[ldap.SecurityProtocol(f.SecurityProtocol)]
	c.Data["AuthSources"] = authSources
	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = db.SMTPAuths

	hasTLS := false
	var config interface{}
	switch db.LoginType(f.Type) {
	case db.LoginLDAP, db.LoginDLDAP:
		config = parseLDAPConfig(f)
		hasTLS = ldap.SecurityProtocol(f.SecurityProtocol) > ldap.SecurityProtocolUnencrypted
	case db.LoginSMTP:
		config = parseSMTPConfig(f)
		hasTLS = true
	case db.LoginPAM:
		config = &db.PAMConfig{
			ServiceName: f.PAMServiceName,
		}
	case db.LoginGitHub:
		config = &db.GitHubConfig{
			APIEndpoint: strings.TrimSuffix(f.GitHubAPIEndpoint, "/") + "/",
		}
	default:
		c.Status(http.StatusBadRequest)
		return
	}
	c.Data["HasTLS"] = hasTLS

	if c.HasError() {
		c.Success(AUTH_NEW)
		return
	}

	source, err := db.LoginSources.Create(db.CreateLoginSourceOpts{
		Type:      db.LoginType(f.Type),
		Name:      f.Name,
		Activated: f.IsActive,
		Default:   f.IsDefault,
		Config:    config,
	})
	if err != nil {
		if db.IsErrLoginSourceAlreadyExist(err) {
			c.FormErr("Name")
			c.RenderWithErr(c.Tr("admin.auths.login_source_exist", f.Name), AUTH_NEW, f)
		} else {
			c.Error(err, "create login source")
		}
		return
	}

	if source.IsDefault {
		err = db.LoginSources.ResetNonDefault(source)
		if err != nil {
			c.Error(err, "reset non-default login sources")
			return
		}
	}

	log.Trace("Authentication created by admin(%s): %s", c.User.Name, f.Name)

	c.Flash.Success(c.Tr("admin.auths.new_success", f.Name))
	c.Redirect(conf.Server.Subpath + "/admin/auths")
}

func EditAuthSource(c *context.Context) {
	c.Title("admin.auths.edit")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = db.SMTPAuths

	source, err := db.LoginSources.GetByID(c.ParamsInt64(":authid"))
	if err != nil {
		c.Error(err, "get login source by ID")
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

	c.Data["SMTPAuths"] = db.SMTPAuths

	source, err := db.LoginSources.GetByID(c.ParamsInt64(":authid"))
	if err != nil {
		c.Error(err, "get login source by ID")
		return
	}
	c.Data["Source"] = source
	c.Data["HasTLS"] = source.HasTLS()

	if c.HasError() {
		c.Success(AUTH_EDIT)
		return
	}

	var config interface{}
	switch db.LoginType(f.Type) {
	case db.LoginLDAP, db.LoginDLDAP:
		config = parseLDAPConfig(f)
	case db.LoginSMTP:
		config = parseSMTPConfig(f)
	case db.LoginPAM:
		config = &db.PAMConfig{
			ServiceName: f.PAMServiceName,
		}
	case db.LoginGitHub:
		config = &db.GitHubConfig{
			APIEndpoint: strings.TrimSuffix(f.GitHubAPIEndpoint, "/") + "/",
		}
	default:
		c.Status(http.StatusBadRequest)
		return
	}

	source.Name = f.Name
	source.IsActived = f.IsActive
	source.IsDefault = f.IsDefault
	source.Config = config
	if err := db.LoginSources.Save(source); err != nil {
		c.Error(err, "update login source")
		return
	}

	if source.IsDefault {
		err = db.LoginSources.ResetNonDefault(source)
		if err != nil {
			c.Error(err, "reset non-default login sources")
			return
		}
	}

	log.Trace("Authentication changed by admin '%s': %d", c.User.Name, source.ID)

	c.Flash.Success(c.Tr("admin.auths.update_success"))
	c.Redirect(conf.Server.Subpath + "/admin/auths/" + com.ToStr(f.ID))
}

func DeleteAuthSource(c *context.Context) {
	id := c.ParamsInt64(":authid")
	if err := db.LoginSources.DeleteByID(id); err != nil {
		if db.IsErrLoginSourceInUse(err) {
			c.Flash.Error(c.Tr("admin.auths.still_in_used"))
		} else {
			c.Flash.Error(fmt.Sprintf("DeleteSource: %v", err))
		}
		c.JSONSuccess(map[string]interface{}{
			"redirect": conf.Server.Subpath + "/admin/auths/" + c.Params(":authid"),
		})
		return
	}
	log.Trace("Authentication deleted by admin(%s): %d", c.User.Name, id)

	c.Flash.Success(c.Tr("admin.auths.deletion_success"))
	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/admin/auths",
	})
}
