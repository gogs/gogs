// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/http"

	//"github.com/martini-contrib/binding"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/utils/log"
)

func Dashboard(r render.Render, data base.TmplData, session sessions.Session) {
	if !IsSignedIn(session) {
		// todo : direct to logout
		r.Redirect("/")
		return
	}

	data["IsSigned"] = true
	data["SignedUserId"] = SignedInId(session)
	data["SignedUserName"] = SignedInName(session)
	data["PageIsUserDashboard"] = true
	data["Avatar"] = SignedInUser(session).Avatar

	data["Title"] = "Dashboard"
	r.HTML(200, "user/dashboard", data)
}

func Profile(r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Profile"

	data["IsSigned"] = IsSignedIn(session)
	// TODO: Need to check view self or others.
	user := SignedInUser(session)
	data["Avatar"] = user.Avatar
	data["Username"] = user.Name
	r.HTML(200, "user/profile", data)
}

func IsSignedIn(session sessions.Session) bool {
	return SignedInId(session) > 0
}

func SignedInId(session sessions.Session) int64 {
	userId := session.Get("userId")
	if userId == nil {
		return 0
	}
	if s, ok := userId.(int64); ok {
		return s
	}
	return 0
}

func SignedInName(session sessions.Session) string {
	userName := session.Get("userName")
	if userName == nil {
		return ""
	}
	if s, ok := userName.(string); ok {
		return s
	}
	return ""
}

func SignedInUser(session sessions.Session) *models.User {
	id := SignedInId(session)
	if id <= 0 {
		return nil
	}

	user, err := models.GetUserById(id)
	if err != nil {
		log.Error("user.SignedInUser: %v", err)
		return nil
	}
	return user
}

func SignIn(form auth.LogInForm, data base.TmplData, req *http.Request, r render.Render, session sessions.Session) {
	// if logged, do not show login page
	if IsSignedIn(session) {
		r.Redirect("/")
		return
	}

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
		log.Error("user.SignIn: %v", data)
		r.HTML(500, "base/error", nil)
		return
	}

	// login success
	session.Set("userId", user.Id)
	session.Set("userName", user.Name)
	r.Redirect("/")
}

func SignOut(r render.Render, session sessions.Session) {
	if !IsSignedIn(session) {
		r.Redirect("/")
		return
	}

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
			r.HTML(500, "base/error", nil)
		}
		return
	}

	r.Redirect("/user/login")
}

func Delete(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "user/delete", map[string]interface{}{
			"Title": "Delete user",
		})
		return
	}

	u := &models.User{}
	err := models.DeleteUser(u)
	r.HTML(403, "status/403", map[string]interface{}{
		"Title": fmt.Sprintf("%v", err),
	})
}
