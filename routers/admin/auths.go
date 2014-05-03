package admin

import (
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gpmgo/gopm/log"
)

func NewAuthSource(ctx *middleware.Context) {
	ctx.Data["Title"] = "New Authentication"
	ctx.Data["PageIsAuths"] = true
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

func EditAuthSource(ctx *middleware.Context) {
}

func EditAuthSourcePost(ctx *middleware.Context) {
}

func DeleteAuthSource(ctx *middleware.Context) {
}
