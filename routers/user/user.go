// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func Dashboard(r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Dashboard"
	data["PageIsUserDashboard"] = true
	repos, err := models.GetRepositories(&models.User{Id: auth.SignedInId(session)})
	if err != nil {
		log.Handle(200, "user.Dashboard", data, r, err)
		return
	}
	data["MyRepos"] = repos
	r.HTML(200, "user/dashboard", data)
}

func Profile(params martini.Params, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Profile"

	// TODO: Need to check view self or others.
	user, err := models.GetUserByName(params["username"])
	if err != nil {
		log.Handle(200, "user.Profile", data, r, err)
		return
	}

	data["Owner"] = user
	feeds, err := models.GetFeeds(user.Id, 0, true)
	if err != nil {
		log.Handle(200, "user.Profile", data, r, err)
		return
	}
	data["Feeds"] = feeds
	r.HTML(200, "user/profile", data)
}

func SignIn(form auth.LogInForm, data base.TmplData, req *http.Request, r render.Render, session sessions.Session) {
	data["Title"] = "Log In"

	if req.Method == "GET" {
		r.HTML(200, "user/signin", data)
		return
	}

	if hasErr, ok := data["HasError"]; ok && hasErr.(bool) {
		r.HTML(200, "user/signin", data)
		return
	}

	user, err := models.LoginUserPlain(form.UserName, form.Password)
	if err != nil {
		if err.Error() == models.ErrUserNotExist.Error() {
			data["HasError"] = true
			data["ErrorMsg"] = "Username or password is not correct"
			auth.AssignForm(form, data)
			r.HTML(200, "user/signin", data)
			return
		}

		log.Handle(200, "user.SignIn", data, r, err)
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

func SignUp(form auth.RegisterForm, data base.TmplData, req *http.Request, r render.Render) {
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
			log.Handle(200, "user.SignUp", data, r, err)
		}
		return
	}

	r.Redirect("/user/login")
}

func Delete(data base.TmplData, req *http.Request, session sessions.Session, r render.Render) {
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
			log.Handle(200, "user.Delete", data, r, err)
			return
		}
	}

	r.HTML(200, "user/delete", data)
}

func Feeds(form auth.FeedsForm, r render.Render) {
	actions, err := models.GetFeeds(form.UserId, form.Offset, false)
	if err != nil {
		r.JSON(500, err)
	}
	r.JSON(200, actions)
}
