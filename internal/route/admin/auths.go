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

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
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
	c.Data["Sources"], err = database.LoginSources.List(c.Req.Context(), database.ListLoginSourceOptions{})
	if err != nil {
		c.Error(err, "list login sources")
		return
	}

	c.Data["Total"] = database.LoginSources.Count(c.Req.Context())
	c.Success(AUTHS)
}

type dropdownItem struct {
	Name string
	Type any
}

var (
	authSources = []dropdownItem{
		{auth.Name(auth.LDAP), auth.LDAP},
		{auth.Name(auth.DLDAP), auth.DLDAP},
		{auth.Name(auth.SMTP), auth.SMTP},
		{auth.Name(auth.PAM), auth.PAM},
		{auth.Name(auth.GitHub), auth.GitHub},
	}
	securityProtocols = []dropdownItem{
		{ldap.SecurityProtocolName(ldap.SecurityProtocolUnencrypted), ldap.SecurityProtocolUnencrypted},
		{ldap.SecurityProtocolName(ldap.SecurityProtocolLDAPS), ldap.SecurityProtocolLDAPS},
		{ldap.SecurityProtocolName(ldap.SecurityProtocolStartTLS), ldap.SecurityProtocolStartTLS},
	}
)

func NewAuthSource(c *context.Context) {
	c.Title("admin.auths.new")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["type"] = auth.LDAP
	c.Data["CurrentTypeName"] = auth.Name(auth.LDAP)
	c.Data["CurrentSecurityProtocol"] = ldap.SecurityProtocolName(ldap.SecurityProtocolUnencrypted)
	c.Data["smtp_auth"] = "PLAIN"
	c.Data["is_active"] = true
	c.Data["is_default"] = true
	c.Data["AuthSources"] = authSources
	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = smtp.AuthTypes
	c.Success(AUTH_NEW)
}

func parseLDAPConfig(f form.Authentication) *ldap.Config {
	return &ldap.Config{
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
	}
}

func parseSMTPConfig(f form.Authentication) *smtp.Config {
	return &smtp.Config{
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

	c.Data["CurrentTypeName"] = auth.Name(auth.Type(f.Type))
	c.Data["CurrentSecurityProtocol"] = ldap.SecurityProtocolName(ldap.SecurityProtocol(f.SecurityProtocol))
	c.Data["AuthSources"] = authSources
	c.Data["SecurityProtocols"] = securityProtocols
	c.Data["SMTPAuths"] = smtp.AuthTypes

	hasTLS := false
	var config any
	switch auth.Type(f.Type) {
	case auth.LDAP, auth.DLDAP:
		config = parseLDAPConfig(f)
		hasTLS = ldap.SecurityProtocol(f.SecurityProtocol) > ldap.SecurityProtocolUnencrypted
	case auth.SMTP:
		config = parseSMTPConfig(f)
		hasTLS = true
	case auth.PAM:
		config = &pam.Config{
			ServiceName: f.PAMServiceName,
		}
	case auth.GitHub:
		config = &github.Config{
			APIEndpoint: strings.TrimSuffix(f.GitHubAPIEndpoint, "/") + "/",
			SkipVerify:  f.SkipVerify,
		}
		hasTLS = true
	default:
		c.Status(http.StatusBadRequest)
		return
	}
	c.Data["HasTLS"] = hasTLS

	if c.HasError() {
		c.Success(AUTH_NEW)
		return
	}

	source, err := database.LoginSources.Create(c.Req.Context(),
		database.CreateLoginSourceOptions{
			Type:      auth.Type(f.Type),
			Name:      f.Name,
			Activated: f.IsActive,
			Default:   f.IsDefault,
			Config:    config,
		},
	)
	if err != nil {
		if database.IsErrLoginSourceAlreadyExist(err) {
			c.FormErr("Name")
			c.RenderWithErr(c.Tr("admin.auths.login_source_exist", f.Name), AUTH_NEW, f)
		} else {
			c.Error(err, "create login source")
		}
		return
	}

	if source.IsDefault {
		err = database.LoginSources.ResetNonDefault(c.Req.Context(), source)
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
	c.Data["SMTPAuths"] = smtp.AuthTypes

	source, err := database.LoginSources.GetByID(c.Req.Context(), c.ParamsInt64(":authid"))
	if err != nil {
		c.Error(err, "get login source by ID")
		return
	}
	c.Data["Source"] = source
	c.Data["HasTLS"] = source.Provider.HasTLS()

	c.Success(AUTH_EDIT)
}

func EditAuthSourcePost(c *context.Context, f form.Authentication) {
	c.Title("admin.auths.edit")
	c.PageIs("Admin")
	c.PageIs("AdminAuthentications")

	c.Data["SMTPAuths"] = smtp.AuthTypes

	source, err := database.LoginSources.GetByID(c.Req.Context(), c.ParamsInt64(":authid"))
	if err != nil {
		c.Error(err, "get login source by ID")
		return
	}
	c.Data["Source"] = source
	c.Data["HasTLS"] = source.Provider.HasTLS()

	if c.HasError() {
		c.Success(AUTH_EDIT)
		return
	}

	var provider auth.Provider
	switch auth.Type(f.Type) {
	case auth.LDAP:
		provider = ldap.NewProvider(false, parseLDAPConfig(f))
	case auth.DLDAP:
		provider = ldap.NewProvider(true, parseLDAPConfig(f))
	case auth.SMTP:
		provider = smtp.NewProvider(parseSMTPConfig(f))
	case auth.PAM:
		provider = pam.NewProvider(&pam.Config{
			ServiceName: f.PAMServiceName,
		})
	case auth.GitHub:
		provider = github.NewProvider(&github.Config{
			APIEndpoint: strings.TrimSuffix(f.GitHubAPIEndpoint, "/") + "/",
			SkipVerify:  f.SkipVerify,
		})
	default:
		c.Status(http.StatusBadRequest)
		return
	}

	source.Name = f.Name
	source.IsActived = f.IsActive
	source.IsDefault = f.IsDefault
	source.Provider = provider
	if err := database.LoginSources.Save(c.Req.Context(), source); err != nil {
		c.Error(err, "update login source")
		return
	}

	if source.IsDefault {
		err = database.LoginSources.ResetNonDefault(c.Req.Context(), source)
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
	if err := database.LoginSources.DeleteByID(c.Req.Context(), id); err != nil {
		if database.IsErrLoginSourceInUse(err) {
			c.Flash.Error(c.Tr("admin.auths.still_in_used"))
		} else {
			c.Flash.Error(fmt.Sprintf("DeleteSource: %v", err))
		}
		c.JSONSuccess(map[string]any{
			"redirect": conf.Server.Subpath + "/admin/auths/" + c.Params(":authid"),
		})
		return
	}
	log.Trace("Authentication deleted by admin(%s): %d", c.User.Name, id)

	c.Flash.Success(c.Tr("admin.auths.deletion_success"))
	c.JSONSuccess(map[string]any{
		"redirect": conf.Server.Subpath + "/admin/auths",
	})
}
