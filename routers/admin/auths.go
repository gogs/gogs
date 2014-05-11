// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"strings"

	"github.com/go-martini/martini"
	"github.com/go-xorm/core"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func NewAuthSource(ctx *middleware.Context) {
	ctx.Data["Title"] = "New Authentication"
	ctx.Data["PageIsAuths"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths
	ctx.HTML(200, "admin/auths/new")
}

func NewAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = "New Authentication"
	ctx.Data["PageIsAuths"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	if ctx.HasError() {
		ctx.HTML(200, "admin/auths/new")
		return
	}

	var u core.Conversion
	switch form.Type {
	case models.LT_LDAP:
		u = &models.LDAPConfig{
			Ldapsource: ldap.Ldapsource{
				Host:         form.Host,
				Port:         form.Port,
				BaseDN:       form.BaseDN,
				Attributes:   form.Attributes,
				Filter:       form.Filter,
				MsAdSAFormat: form.MsAdSA,
				Enabled:      true,
				Name:         form.AuthName,
			},
		}
	case models.LT_SMTP:
		u = &models.SMTPConfig{
			Auth: form.SmtpAuth,
			Host: form.Host,
			Port: form.Port,
			TLS:  form.Tls,
		}
	default:
		ctx.Error(400)
		return
	}

	var source = &models.LoginSource{
		Type:              form.Type,
		Name:              form.AuthName,
		IsActived:         true,
		AllowAutoRegister: form.AllowAutoRegister,
		Cfg:               u,
	}

	if err := models.AddSource(source); err != nil {
		ctx.Handle(500, "admin.auths.NewAuth", err)
		return
	}

	log.Trace("%s Authentication created by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, strings.ToLower(form.AuthName))

	ctx.Redirect("/admin/auths")
}

func EditAuthSource(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Edit Authentication"
	ctx.Data["PageIsAuths"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	id, err := base.StrTo(params["authid"]).Int64()
	if err != nil {
		ctx.Handle(404, "admin.auths.EditAuthSource", err)
		return
	}
	u, err := models.GetLoginSourceById(id)
	if err != nil {
		ctx.Handle(500, "admin.user.EditUser", err)
		return
	}
	ctx.Data["Source"] = u
	ctx.HTML(200, "admin/auths/edit")
}

func EditAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = "Edit Authentication"
	ctx.Data["PageIsAuths"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.Data["SMTPAuths"] = models.SMTPAuths

	if ctx.HasError() {
		ctx.HTML(200, "admin/auths/edit")
		return
	}

	var config core.Conversion
	switch form.Type {
	case models.LT_LDAP:
		config = &models.LDAPConfig{
			Ldapsource: ldap.Ldapsource{
				Host:         form.Host,
				Port:         form.Port,
				BaseDN:       form.BaseDN,
				Attributes:   form.Attributes,
				Filter:       form.Filter,
				MsAdSAFormat: form.MsAdSA,
				Enabled:      true,
				Name:         form.AuthName,
			},
		}
	case models.LT_SMTP:
		config = &models.SMTPConfig{
			Auth: form.SmtpAuth,
			Host: form.Host,
			Port: form.Port,
			TLS:  form.Tls,
		}
	default:
		ctx.Error(400)
		return
	}

	u := models.LoginSource{
		Name:              form.AuthName,
		IsActived:         form.IsActived,
		Type:              form.Type,
		AllowAutoRegister: form.AllowAutoRegister,
		Cfg:               config,
	}

	if err := models.UpdateSource(&u); err != nil {
		ctx.Handle(500, "admin.auths.EditAuth", err)
		return
	}

	log.Trace("%s Authentication changed by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, strings.ToLower(form.AuthName))

	ctx.Redirect("/admin/auths")
}

func DeleteAuthSource(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Delete Authentication"
	ctx.Data["PageIsAuths"] = true

	id, err := base.StrTo(params["authid"]).Int64()
	if err != nil {
		ctx.Handle(404, "admin.auths.DeleteAuth", err)
		return
	}

	a, err := models.GetLoginSourceById(id)
	if err != nil {
		ctx.Handle(500, "admin.auths.DeleteAuth", err)
		return
	}

	if err = models.DelLoginSource(a); err != nil {
		switch err {
		case models.ErrAuthenticationUserUsed:
			ctx.Flash.Error("This authentication still has used by some users, you should move them and then delete again.")
			ctx.Redirect("/admin/auths/" + params["authid"])
		default:
			ctx.Handle(500, "admin.auths.DeleteAuth", err)
		}
		return
	}
	log.Trace("%s Authentication deleted by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, ctx.User.LowerName)

	ctx.Redirect("/admin/auths")
}
