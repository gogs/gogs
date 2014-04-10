// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"strings"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func NewUser(ctx *middleware.Context) {
	ctx.Data["Title"] = "New Account"
	ctx.Data["PageIsUsers"] = true
	ctx.HTML(200, "admin/users/new")
}

func NewUserPost(ctx *middleware.Context, form auth.RegisterForm) {
	ctx.Data["Title"] = "New Account"
	ctx.Data["PageIsUsers"] = true

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
			ctx.Handle(500, "admin.user.NewUser", err)
		}
		return
	}

	log.Trace("%s User created by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, strings.ToLower(form.UserName))

	ctx.Redirect("/admin/users")
}

func EditUser(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Edit Account"
	ctx.Data["PageIsUsers"] = true

	uid, err := base.StrTo(params["userid"]).Int()
	if err != nil {
		ctx.Handle(404, "admin.user.EditUser", err)
		return
	}

	u, err := models.GetUserById(int64(uid))
	if err != nil {
		ctx.Handle(500, "admin.user.EditUser", err)
		return
	}

	ctx.Data["User"] = u
	ctx.HTML(200, "admin/users/edit")
}

func EditUserPost(ctx *middleware.Context, params martini.Params, form auth.AdminEditUserForm) {
	ctx.Data["Title"] = "Edit Account"
	ctx.Data["PageIsUsers"] = true

	uid, err := base.StrTo(params["userid"]).Int()
	if err != nil {
		ctx.Handle(404, "admin.user.EditUser", err)
		return
	}

	u, err := models.GetUserById(int64(uid))
	if err != nil {
		ctx.Handle(500, "admin.user.EditUser", err)
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
		ctx.Handle(500, "admin.user.EditUser", err)
		return
	}
	log.Trace("%s User profile updated by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, ctx.User.LowerName)

	ctx.Data["User"] = u
	ctx.Flash.Success("Account profile has been successfully updated.")
	ctx.Redirect("/admin/users/" + params["userid"])
}

func DeleteUser(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Delete Account"
	ctx.Data["PageIsUsers"] = true

	log.Info("delete")
	uid, err := base.StrTo(params["userid"]).Int()
	if err != nil {
		ctx.Handle(404, "admin.user.EditUser", err)
		return
	}

	u, err := models.GetUserById(int64(uid))
	if err != nil {
		ctx.Handle(500, "admin.user.EditUser", err)
		return
	}

	if err = models.DeleteUser(u); err != nil {
		switch err {
		case models.ErrUserOwnRepos:
			ctx.Flash.Error("This account still has ownership of repository, owner has to delete or transfer them first.")
			ctx.Redirect("/admin/users/" + params["userid"])
		default:
			ctx.Handle(500, "admin.user.DeleteUser", err)
		}
		return
	}
	log.Trace("%s User deleted by admin(%s): %s", ctx.Req.RequestURI,
		ctx.User.LowerName, ctx.User.LowerName)

	ctx.Redirect("/admin/users")
}
