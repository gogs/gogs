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
	r.HTML(200, "user/dashboard", data)
}

func Profile(params martini.Params, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Profile"

	// TODO: Need to check view self or others.
	user, err := models.GetUserByName(params["username"])
	if err != nil {
		data["ErrorMsg"] = err
		log.Error("user.Profile: %v", err)
		r.HTML(200, "base/error", data)
		return
	}

	data["Avatar"] = user.Avatar
	data["Username"] = user.Name
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

		data["ErrorMsg"] = err
		log.Error("user.SignIn: %v", err)
		r.HTML(200, "base/error", data)
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
			data["ErrorMsg"] = err
			log.Error("user.SignUp: %v", data)
			r.HTML(200, "base/error", nil)
		}
		return
	}

	r.Redirect("/user/login")
}

// TODO: unfinished
func Delete(data base.TmplData, req *http.Request, r render.Render) {
	data["Title"] = "Delete user"

	if req.Method == "GET" {
		r.HTML(200, "user/delete", data)
		return
	}

	u := &models.User{}
	err := models.DeleteUser(u)
	data["ErrorMsg"] = err
	log.Error("user.Delete: %v", data)
	r.HTML(200, "base/error", nil)
}
