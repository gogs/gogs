// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = "Dashboard"
	ctx.Data["PageIsUserDashboard"] = true
	repos, err := models.GetRepositories(&models.User{Id: ctx.User.Id})
	if err != nil {
		ctx.Handle(200, "user.Dashboard", err)
		return
	}
	ctx.Data["MyRepos"] = repos

	feeds, err := models.GetFeeds(ctx.User.Id, 0, false)
	if err != nil {
		ctx.Handle(200, "user.Dashboard", err)
		return
	}
	ctx.Data["Feeds"] = feeds
	ctx.Render.HTML(200, "user/dashboard", ctx.Data)
}

func Profile(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Profile"

	// TODO: Need to check view self or others.
	user, err := models.GetUserByName(params["username"])
	if err != nil {
		ctx.Handle(200, "user.Profile", err)
		return
	}

	ctx.Data["Owner"] = user

	tab := ctx.Query("tab")
	ctx.Data["TabName"] = tab

	switch tab {
	case "activity":
		feeds, err := models.GetFeeds(user.Id, 0, true)
		if err != nil {
			ctx.Handle(200, "user.Profile", err)
			return
		}
		ctx.Data["Feeds"] = feeds
	default:

	}

	ctx.Render.HTML(200, "user/profile", ctx.Data)
}

func SignIn(form auth.LogInForm, ctx *middleware.Context, r render.Render, session sessions.Session) {
	ctx.Data["Title"] = "Log In"

	if ctx.Req.Method == "GET" {
		ctx.Render.HTML(200, "user/signin", ctx.Data)
		return
	}

	if hasErr, ok := ctx.Data["HasError"]; ok && hasErr.(bool) {
		ctx.Render.HTML(200, "user/signin", ctx.Data)
		return
	}

	user, err := models.LoginUserPlain(form.UserName, form.Password)
	if err != nil {
		if err.Error() == models.ErrUserNotExist.Error() {
			ctx.Data["HasError"] = true
			ctx.Data["ErrorMsg"] = "Username or password is not correct"
			auth.AssignForm(form, ctx.Data)
			ctx.Render.HTML(200, "user/signin", ctx.Data)
			return
		}

		ctx.Handle(200, "user.SignIn", err)
		return
	}

	session.Set("userId", user.Id)
	session.Set("userName", user.Name)
	r.Redirect("/")
}

func SignOut(r render.Render, session sessions.Session) {
	session.Delete("userId")
	session.Delete("userName")
	r.Redirect("/")
}

func SignUp(form auth.RegisterForm, ctx *middleware.Context, data base.TmplData, req *http.Request, r render.Render) {
	data["Title"] = "Sign Up"
	data["PageIsSignUp"] = true

	if req.Method == "GET" {
		r.HTML(200, "user/signup", data)
		return
	}

	if form.Password != form.RetypePasswd {
		data["HasError"] = true
		data["Err_Password"] = true
		data["Err_RetypePasswd"] = true
		data["ErrorMsg"] = "Password and re-type password are not same"
		auth.AssignForm(form, data)
	}

	if hasErr, ok := data["HasError"]; ok && hasErr.(bool) {
		r.HTML(200, "user/signup", data)
		return
	}

	u := &models.User{
		Name:   form.UserName,
		Email:  form.Email,
		Passwd: form.Password,
	}

	if err := models.RegisterUser(u); err != nil {
		data["HasError"] = true
		auth.AssignForm(form, data)

		switch err.Error() {
		case models.ErrUserAlreadyExist.Error():
			data["Err_Username"] = true
			data["ErrorMsg"] = "Username has been already taken"
			r.HTML(200, "user/signup", data)
		case models.ErrEmailAlreadyUsed.Error():
			data["Err_Email"] = true
			data["ErrorMsg"] = "E-mail address has been already used"
			r.HTML(200, "user/signup", data)
		default:
			ctx.Handle(200, "user.SignUp", err)
		}
		return
	}

	r.Redirect("/user/login")
}

func Delete(data base.TmplData, ctx *middleware.Context, req *http.Request, session sessions.Session, r render.Render) {
	data["Title"] = "Delete Account"

	if req.Method == "GET" {
		r.HTML(200, "user/delete", data)
		return
	}

	id := auth.SignedInId(session)
	u := &models.User{Id: id}
	if err := models.DeleteUser(u); err != nil {
		data["HasError"] = true
		switch err.Error() {
		case models.ErrUserOwnRepos.Error():
			data["ErrorMsg"] = "Your account still have ownership of repository, you have to delete or transfer them first."
		default:
			ctx.Handle(200, "user.Delete", err)
			return
		}
	}

	r.HTML(200, "user/delete", data)
}

const (
	feedTpl = `<i class="icon fa fa-%s"></i>
                        <div class="info"><span class="meta">%s</span><br>%s</div>`
)

func Feeds(form auth.FeedsForm, r render.Render) {
	actions, err := models.GetFeeds(form.UserId, form.Page*20, false)
	if err != nil {
		r.JSON(500, err)
	}

	feeds := make([]string, len(actions))
	for i := range actions {
		feeds[i] = fmt.Sprintf(feedTpl, base.ActionIcon(actions[i].OpType),
			base.TimeSince(actions[i].Created), base.ActionDesc(actions[i]))
	}
	r.JSON(200, &feeds)
}
