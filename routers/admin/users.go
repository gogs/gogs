// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"math"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	USERS     base.TplName = "admin/user/list"
	USER_NEW  base.TplName = "admin/user/new"
	USER_EDIT base.TplName = "admin/user/edit"
)

func pagination(ctx *middleware.Context, count int64, pageNum int) int {
	p := ctx.QueryInt("p")
	if p < 1 {
		p = 1
	}
	curCount := int64((p-1)*pageNum + pageNum)
	if curCount >= count {
		p = int(math.Ceil(float64(count) / float64(pageNum)))
	} else {
		ctx.Data["NextPageNum"] = p + 1
	}
	if p > 1 {
		ctx.Data["LastPageNum"] = p - 1
	}
	return p
}

func Users(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.users")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminUsers"] = true

	pageNum := 50
	p := pagination(ctx, models.CountUsers(), pageNum)

	users, err := models.GetUsers(pageNum, (p-1)*pageNum)
	if err != nil {
		ctx.Handle(500, "GetUsers", err)
		return
	}
	ctx.Data["Users"] = users
	ctx.HTML(200, USERS)
}

func NewUser(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.users.new_account")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminUsers"] = true

	auths, err := models.GetAuths()
	if err != nil {
		ctx.Handle(500, "GetAuths", err)
		return
	}
	ctx.Data["LoginSources"] = auths
	ctx.HTML(200, USER_NEW)
}

func NewUserPost(ctx *middleware.Context, form auth.RegisterForm) {
	ctx.Data["Title"] = ctx.Tr("admin.users.new_account")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminUsers"] = true

	if ctx.HasError() {
		ctx.HTML(200, USER_NEW)
		return
	}

	if form.Password != form.Retype {
		ctx.Data["Err_Password"] = true
		ctx.RenderWithErr(ctx.Tr("form.password_not_match"), USER_NEW, &form)
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
		tp, _ := com.StrTo(fields[0]).Int()
		u.LoginType = models.LoginType(tp)
		u.LoginSource, _ = com.StrTo(fields[1]).Int64()
		u.LoginName = form.LoginName
	}

	if err := models.CreateUser(u); err != nil {
		switch {
		case models.IsErrUserAlreadyExist(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), USER_NEW, &form)
		case models.IsErrEmailAlreadyUsed(err):
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), USER_NEW, &form)
		case models.IsErrNameReserved(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_reserved", err.(models.ErrNameReserved).Name), USER_NEW, &form)
		case models.IsErrNamePatternNotAllowed(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), USER_NEW, &form)
		default:
			ctx.Handle(500, "CreateUser", err)
		}
		return
	}
	log.Trace("Account created by admin(%s): %s", ctx.User.Name, u.Name)
	ctx.Redirect(setting.AppSubUrl + "/admin/users")
}

func EditUser(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.users.edit_account")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminUsers"] = true

	uid := com.StrTo(ctx.Params(":userid")).MustInt64()
	if uid == 0 {
		ctx.Handle(404, "EditUser", nil)
		return
	}

	u, err := models.GetUserByID(uid)
	if err != nil {
		ctx.Handle(500, "GetUserByID", err)
		return
	}

	ctx.Data["User"] = u
	auths, err := models.GetAuths()
	if err != nil {
		ctx.Handle(500, "GetAuths", err)
		return
	}
	ctx.Data["LoginSources"] = auths
	ctx.HTML(200, USER_EDIT)
}

func EditUserPost(ctx *middleware.Context, form auth.AdminEditUserForm) {
	ctx.Data["Title"] = ctx.Tr("admin.users.edit_account")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminUsers"] = true

	uid := com.StrTo(ctx.Params(":userid")).MustInt64()
	if uid == 0 {
		ctx.Handle(404, "EditUser", nil)
		return
	}

	u, err := models.GetUserByID(uid)
	if err != nil {
		ctx.Handle(500, "GetUserById", err)
		return
	}
	ctx.Data["User"] = u

	if ctx.HasError() {
		ctx.HTML(200, USER_EDIT)
		return
	}

	// FIXME: need password length check
	if len(form.Password) > 0 {
		u.Passwd = form.Password
		u.Salt = models.GetUserSalt()
		u.EncodePasswd()
	}

	u.FullName = form.FullName
	u.Email = form.Email
	u.Website = form.Website
	u.Location = form.Location
	if len(form.Avatar) == 0 {
		form.Avatar = form.Email
	}
	u.Avatar = base.EncodeMd5(form.Avatar)
	u.AvatarEmail = form.Avatar
	u.IsActive = form.Active
	u.IsAdmin = form.Admin
	u.AllowGitHook = form.AllowGitHook

	if err := models.UpdateUser(u); err != nil {
		if models.IsErrEmailAlreadyUsed(err) {
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), USER_EDIT, &form)
		} else {
			ctx.Handle(500, "UpdateUser", err)
		}
		return
	}
	log.Trace("Account profile updated by admin(%s): %s", ctx.User.Name, u.Name)
	ctx.Flash.Success(ctx.Tr("admin.users.update_profile_success"))
	ctx.Redirect(setting.AppSubUrl + "/admin/users/" + ctx.Params(":userid"))
}

func DeleteUser(ctx *middleware.Context) {
	uid := com.StrTo(ctx.Params(":userid")).MustInt64()
	if uid == 0 {
		ctx.Handle(404, "DeleteUser", nil)
		return
	}

	u, err := models.GetUserByID(uid)
	if err != nil {
		ctx.Handle(500, "GetUserByID", err)
		return
	}

	if err = models.DeleteUser(u); err != nil {
		switch {
		case models.IsErrUserOwnRepos(err):
			ctx.Flash.Error(ctx.Tr("admin.users.still_own_repo"))
			ctx.Redirect(setting.AppSubUrl + "/admin/users/" + ctx.Params(":userid"))
		case models.IsErrUserHasOrgs(err):
			ctx.Flash.Error(ctx.Tr("admin.users.still_has_org"))
			ctx.Redirect(setting.AppSubUrl + "/admin/users/" + ctx.Params(":userid"))
		default:
			ctx.Handle(500, "DeleteUser", err)
		}
		return
	}
	log.Trace("Account deleted by admin(%s): %s", ctx.User.Name, u.Name)
	ctx.Redirect(setting.AppSubUrl + "/admin/users")
}
