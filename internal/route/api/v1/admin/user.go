// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/route/api/v1/user"
)

func parseLoginSource(c *context.APIContext, sourceID int64) {
	if sourceID == 0 {
		return
	}

	_, err := database.Handle.LoginSources().GetByID(c.Req.Context(), sourceID)
	if err != nil {
		if database.IsErrLoginSourceNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get login source by ID")
		}
		return
	}
}

func CreateUser(c *context.APIContext, form api.CreateUserOption) {
	parseLoginSource(c, form.SourceID)
	if c.Written() {
		return
	}

	user, err := database.Users.Create(
		c.Req.Context(),
		form.Username,
		form.Email,
		database.CreateUserOptions{
			FullName:    form.FullName,
			Password:    form.Password,
			LoginSource: form.SourceID,
			LoginName:   form.LoginName,
			Activated:   true,
		},
	)
	if err != nil {
		if database.IsErrUserAlreadyExist(err) ||
			database.IsErrEmailAlreadyUsed(err) ||
			database.IsErrNameNotAllowed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "create user")
		}
		return
	}
	log.Trace("Account %q created by admin %q", user.Name, c.User.Name)

	// Send email notification.
	if form.SendNotify && conf.Email.Enabled {
		email.SendRegisterNotifyMail(c.Context.Context, database.NewMailerUser(user))
	}

	c.JSON(http.StatusCreated, user.APIFormat())
}

func EditUser(c *context.APIContext, form api.EditUserOption) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}

	parseLoginSource(c, form.SourceID)
	if c.Written() {
		return
	}

	opts := database.UpdateUserOptions{
		LoginSource:      &form.SourceID,
		LoginName:        &form.LoginName,
		FullName:         &form.FullName,
		Website:          &form.Website,
		Location:         &form.Location,
		MaxRepoCreation:  form.MaxRepoCreation,
		IsActivated:      form.Active,
		IsAdmin:          form.Admin,
		AllowGitHook:     form.AllowGitHook,
		AllowImportLocal: form.AllowImportLocal,
		ProhibitLogin:    nil, // TODO: Add this option to API
	}

	if form.Password != "" {
		opts.Password = &form.Password
	}

	if u.Email != form.Email {
		opts.Email = &form.Email
	}

	err := database.Users.Update(c.Req.Context(), u.ID, opts)
	if err != nil {
		if database.IsErrEmailAlreadyUsed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "update user")
		}
		return
	}
	log.Trace("Account updated by admin %q: %s", c.User.Name, u.Name)

	u, err = database.Users.GetByID(c.Req.Context(), u.ID)
	if err != nil {
		c.Error(err, "get user")
		return
	}
	c.JSONSuccess(u.APIFormat())
}

func DeleteUser(c *context.APIContext) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}

	if err := database.Users.DeleteByID(c.Req.Context(), u.ID, false); err != nil {
		if database.IsErrUserOwnRepos(err) ||
			database.IsErrUserHasOrgs(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "delete user")
		}
		return
	}
	log.Trace("Account deleted by admin(%s): %s", c.User.Name, u.Name)

	c.NoContent()
}

func CreatePublicKey(c *context.APIContext, form api.CreateKeyOption) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}
	user.CreateUserPublicKey(c, form, u.ID)
}
