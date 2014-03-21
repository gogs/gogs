// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func NewUser(ctx *middleware.Context, form auth.RegisterForm) {
	ctx.Data["Title"] = "New Account"

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "admin/users/new")
		return
	}

	if form.Password != form.RetypePasswd {
		ctx.Data["HasError"] = true
		ctx.Data["Err_Password"] = true
		ctx.Data["Err_RetypePasswd"] = true
		ctx.Data["ErrorMsg"] = "Password and re-type password are not same"
		auth.AssignForm(form, ctx.Data)
	}

	if ctx.HasError() {
		ctx.HTML(200, "admin/users/new")
		return
	}

	u := &models.User{
		Name:     form.UserName,
		Email:    form.Email,
		Passwd:   form.Password,
		IsActive: true,
	}

	var err error
	if u, err = models.RegisterUser(u); err != nil {
		switch err {
		case models.ErrUserAlreadyExist:
			ctx.RenderWithErr("Username has been already taken", "admin/users/new", &form)
		case models.ErrEmailAlreadyUsed:
			ctx.RenderWithErr("E-mail address has been already used", "admin/users/new", &form)
		case models.ErrUserNameIllegal:
			ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), "admin/users/new", &form)
		default:
			ctx.Handle(200, "admin.user.NewUser", err)
		}
		return
	}

	log.Trace("%s User created by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, strings.ToLower(form.UserName))

	ctx.Redirect("/admin/users")
}

func EditUser(ctx *middleware.Context, params martini.Params, form auth.AdminEditUserForm) {
	ctx.Data["Title"] = "Edit Account"

	uid, err := base.StrTo(params["userid"]).Int()
	if err != nil {
		ctx.Handle(200, "admin.user.EditUser", err)
		return
	}

	u, err := models.GetUserById(int64(uid))
	if err != nil {
		ctx.Handle(200, "admin.user.EditUser", err)
		return
	}

	if ctx.Req.Method == "GET" {
		ctx.Data["User"] = u
		ctx.HTML(200, "admin/users/edit")
		return
	}

	u.Email = form.Email
	u.Website = form.Website
	u.Location = form.Location
	u.Avatar = base.EncodeMd5(form.Avatar)
	u.AvatarEmail = form.Avatar
	u.IsActive = form.Active == "on"
	u.IsAdmin = form.Admin == "on"
	if err := models.UpdateUser(u); err != nil {
		ctx.Handle(200, "admin.user.EditUser", err)
		return
	}

	ctx.Data["IsSuccess"] = true
	ctx.Data["User"] = u
	ctx.HTML(200, "admin/users/edit")

	log.Trace("%s User profile updated by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, ctx.User.LowerName)
}
