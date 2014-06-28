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

const (
	USER_NEW  base.TplName = "admin/user/new"
	USER_EDIT base.TplName = "admin/user/edit"
)

func NewUser(ctx *middleware.Context) {
	ctx.Data["Title"] = "New Account"
	ctx.Data["PageIsUsers"] = true
	auths, err := models.GetAuths()
	if err != nil {
		ctx.Handle(500, "admin.user.NewUser(GetAuths)", err)
		return
	}
	ctx.Data["LoginSources"] = auths
	ctx.HTML(200, USER_NEW)
}

func NewUserPost(ctx *middleware.Context, form auth.RegisterForm) {
	ctx.Data["Title"] = "New Account"
	ctx.Data["PageIsUsers"] = true

	if ctx.HasError() {
		ctx.HTML(200, USER_NEW)
		return
	}

	if form.Password != form.RetypePasswd {
		ctx.Data["Err_Password"] = true
		ctx.Data["Err_RetypePasswd"] = true
		ctx.RenderWithErr("Password and re-type password are not same.", "admin/users/new", &form)
		return
	}

	u := &models.User{
		Name:      form.UserName,
		Email:     form.Email,
		Passwd:    form.Password,
		IsActive:  true,
		LoginType: models.PLAIN,
	}

	if len(form.LoginType) > 0 {
		// NOTE: need rewrite.
		fields := strings.Split(form.LoginType, "-")
		tp, _ := base.StrTo(fields[0]).Int()
		u.LoginType = models.LoginType(tp)
		u.LoginSource, _ = base.StrTo(fields[1]).Int64()
		u.LoginName = form.LoginName
	}

	var err error
	if u, err = models.CreateUser(u); err != nil {
		switch err {
		case models.ErrUserAlreadyExist:
			ctx.RenderWithErr("Username has been already taken", USER_NEW, &form)
		case models.ErrEmailAlreadyUsed:
			ctx.RenderWithErr("E-mail address has been already used", USER_NEW, &form)
		case models.ErrUserNameIllegal:
			ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), USER_NEW, &form)
		default:
			ctx.Handle(500, "admin.user.NewUser(CreateUser)", err)
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
		ctx.Handle(500, "admin.user.EditUser(GetUserById)", err)
		return
	}

	ctx.Data["User"] = u
	auths, err := models.GetAuths()
	if err != nil {
		ctx.Handle(500, "admin.user.NewUser(GetAuths)", err)
		return
	}
	ctx.Data["LoginSources"] = auths
	ctx.HTML(200, USER_EDIT)
}

func EditUserPost(ctx *middleware.Context, params martini.Params, form auth.AdminEditUserForm) {
	ctx.Data["Title"] = "Edit Account"
	ctx.Data["PageIsUsers"] = true

	uid, err := base.StrTo(params["userid"]).Int()
	if err != nil {
		ctx.Handle(404, "admin.user.EditUserPost", err)
		return
	}

	u, err := models.GetUserById(int64(uid))
	if err != nil {
		ctx.Handle(500, "admin.user.EditUserPost(GetUserById)", err)
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, USER_EDIT)
		return
	}

	u.Email = form.Email
	u.Website = form.Website
	u.Location = form.Location
	u.Avatar = base.EncodeMd5(form.Avatar)
	u.AvatarEmail = form.Avatar
	u.IsActive = form.Active
	u.IsAdmin = form.Admin
	if err := models.UpdateUser(u); err != nil {
		ctx.Handle(500, "admin.user.EditUserPost(UpdateUser)", err)
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

	//log.Info("delete")
	uid, err := base.StrTo(params["userid"]).Int()
	if err != nil {
		ctx.Handle(404, "admin.user.DeleteUser", err)
		return
	}

	u, err := models.GetUserById(int64(uid))
	if err != nil {
		ctx.Handle(500, "admin.user.DeleteUser(GetUserById)", err)
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
