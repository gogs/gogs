// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/routers/api/v1/user"
	to "github.com/gogits/gogs/routers/api/v1/utils"
)

func parseLoginSource(ctx *middleware.Context, u *models.User, sourceID int64, loginName string) {
	if sourceID == 0 {
		return
	}

	source, err := models.GetLoginSourceByID(sourceID)
	if err != nil {
		if models.IsErrAuthenticationNotExist(err) {
			ctx.APIError(422, "", err)
		} else {
			ctx.APIError(500, "GetLoginSourceByID", err)
		}
		return
	}

	u.LoginType = source.Type
	u.LoginSource = source.ID
	u.LoginName = loginName
}

// https://github.com/gogits/go-gogs-client/wiki/Administration-Users#create-a-new-user
func CreateUser(ctx *middleware.Context, form api.CreateUserOption) {
	u := &models.User{
		Name:      form.Username,
		Email:     form.Email,
		Passwd:    form.Password,
		IsActive:  true,
		LoginType: models.LOGIN_PLAIN,
	}

	parseLoginSource(ctx, u, form.SourceID, form.LoginName)
	if ctx.Written() {
		return
	}

	if err := models.CreateUser(u); err != nil {
		if models.IsErrUserAlreadyExist(err) ||
			models.IsErrEmailAlreadyUsed(err) ||
			models.IsErrNameReserved(err) ||
			models.IsErrNamePatternNotAllowed(err) {
			ctx.APIError(422, "", err)
		} else {
			ctx.APIError(500, "CreateUser", err)
		}
		return
	}
	log.Trace("Account created by admin (%s): %s", ctx.User.Name, u.Name)

	// Send e-mail notification.
	if form.SendNotify && setting.MailService != nil {
		mailer.SendRegisterNotifyMail(ctx.Context, u)
	}

	ctx.JSON(201, to.ApiUser(u))
}

// https://github.com/gogits/go-gogs-client/wiki/Administration-Users#edit-an-existing-user
func EditUser(ctx *middleware.Context, form api.EditUserOption) {
	u := user.GetUserByParams(ctx)
	if ctx.Written() {
		return
	}

	parseLoginSource(ctx, u, form.SourceID, form.LoginName)
	if ctx.Written() {
		return
	}

	if len(form.Password) > 0 {
		u.Passwd = form.Password
		u.Salt = models.GetUserSalt()
		u.EncodePasswd()
	}

	u.LoginName = form.LoginName
	u.FullName = form.FullName
	u.Email = form.Email
	u.Website = form.Website
	u.Location = form.Location
	if form.Active != nil {
		u.IsActive = *form.Active
	}
	if form.Admin != nil {
		u.IsAdmin = *form.Admin
	}
	if form.AllowGitHook != nil {
		u.AllowGitHook = *form.AllowGitHook
	}
	if form.AllowImportLocal != nil {
		u.AllowImportLocal = *form.AllowImportLocal
	}

	if err := models.UpdateUser(u); err != nil {
		if models.IsErrEmailAlreadyUsed(err) {
			ctx.APIError(422, "", err)
		} else {
			ctx.APIError(500, "UpdateUser", err)
		}
		return
	}
	log.Trace("Account profile updated by admin (%s): %s", ctx.User.Name, u.Name)

	ctx.JSON(200, to.ApiUser(u))
}

// https://github.com/gogits/go-gogs-client/wiki/Administration-Users#delete-a-user
func DeleteUser(ctx *middleware.Context) {
	u := user.GetUserByParams(ctx)
	if ctx.Written() {
		return
	}

	if err := models.DeleteUser(u); err != nil {
		if models.IsErrUserOwnRepos(err) ||
			models.IsErrUserHasOrgs(err) {
			ctx.APIError(422, "", err)
		} else {
			ctx.APIError(500, "DeleteUser", err)
		}
		return
	}
	log.Trace("Account deleted by admin(%s): %s", ctx.User.Name, u.Name)

	ctx.Status(204)
}

// https://github.com/gogits/go-gogs-client/wiki/Administration-Users#create-a-public-key-for-user
func CreatePublicKey(ctx *middleware.Context, form api.CreateKeyOption) {
	u := user.GetUserByParams(ctx)
	if ctx.Written() {
		return
	}
	user.CreateUserPublicKey(ctx, form, u.Id)
}
