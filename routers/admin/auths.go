package admin

import (
	"strings"

	"github.com/go-martini/martini"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gpmgo/gopm/log"
)

func NewAuthSource(ctx *middleware.Context) {
	ctx.Data["Title"] = "New Authentication"
	ctx.Data["PageIsAuths"] = true
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.HTML(200, "admin/auths/new")
}

func NewAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = "New Authentication"
	ctx.Data["PageIsAuths"] = true

	if ctx.HasError() {
		ctx.HTML(200, "admin/auths/new")
		return
	}

	u := &models.LDAPConfig{
		Ldapsource: ldap.Ldapsource{
			Host:         form.Host,
			Port:         form.Port,
			BaseDN:       form.BaseDN,
			Attributes:   form.Attributes,
			Filter:       form.Filter,
			MsAdSAFormat: form.MsAdSA,
			Enabled:      true,
			Name:         form.Name,
		},
	}

	if err := models.AddLDAPSource(form.Name, u); err != nil {
		switch err {
		default:
			ctx.Handle(500, "admin.auths.NewAuth", err)
		}
		return
	}

	log.Trace("%s Authentication created by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, strings.ToLower(form.Name))

	ctx.Redirect("/admin/auths")
}

func EditAuthSource(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Edit Authentication"
	ctx.Data["PageIsAuths"] = true
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
	ctx.Data["LoginTypes"] = models.LoginTypes
	ctx.HTML(200, "admin/auths/edit")
}

func EditAuthSourcePost(ctx *middleware.Context, form auth.AuthenticationForm) {
	ctx.Data["Title"] = "Edit Authentication"
	ctx.Data["PageIsAuths"] = true

	if ctx.HasError() {
		ctx.HTML(200, "admin/auths/edit")
		return
	}

	u := models.LoginSource{
		Name:      form.Name,
		IsActived: form.IsActived,
		Type:      models.LT_LDAP,
		Cfg: &models.LDAPConfig{
			Ldapsource: ldap.Ldapsource{
				Host:         form.Host,
				Port:         form.Port,
				BaseDN:       form.BaseDN,
				Attributes:   form.Attributes,
				Filter:       form.Filter,
				MsAdSAFormat: form.MsAdSA,
				Enabled:      true,
				Name:         form.Name,
			},
		},
	}

	if err := models.UpdateLDAPSource(&u); err != nil {
		switch err {
		default:
			ctx.Handle(500, "admin.auths.EditAuth", err)
		}
		return
	}

	log.Trace("%s Authentication changed by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, strings.ToLower(form.Name))

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
