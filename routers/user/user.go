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

	data["Title"] = "Dashboard"
	r.HTML(200, "user/dashboard", data)
}

func Profile(r render.Render) {
	r.HTML(200, "user/profile", map[string]interface{}{
		"Title": "Username",
	})
	return
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
		return nil
	}
	return user
}

func SignIn(req *http.Request, r render.Render, session sessions.Session) {
	// if logged, do not show login page
	if IsSignedIn(session) {
		r.Redirect("/")
		return
	}
	var (
		errString string
		account   string
	)
	// if post, do login action
	if req.Method == "POST" {
		account = req.FormValue("account")
		user, err := models.LoginUserPlain(account, req.FormValue("passwd"))
		if err == nil {
			// login success
			session.Set("userId", user.Id)
			session.Set("userName", user.Name)
			r.Redirect("/")
			return
		}
		// login fail
		errString = fmt.Sprintf("%v", err)
	}
	// if get or error post, show login page
	r.HTML(200, "user/signin", map[string]interface{}{
		"Title":   "Log In",
		"Error":   errString,
		"Account": account,
	})
}

func SignUp(form auth.RegisterForm, data base.TmplData, req *http.Request, r render.Render) {
	data["Title"] = "Sign Up"

	if req.Method == "GET" {
		r.HTML(200, "user/signup", data)
		return
	}

	if hasErr, ok := data["HasError"]; ok && hasErr.(bool) {
		r.HTML(200, "user/signup", data)
		return
	}

	//Front-end should do double check of password.
	u := &models.User{
		Name:   form.Username,
		Email:  form.Email,
		Passwd: form.Password,
	}

	if err := models.RegisterUser(u); err != nil {
		if err.Error() == models.ErrUserAlreadyExist.Error() {
			data["HasError"] = true
			data["Err_Username"] = true
			data["ErrorMsg"] = "Username has been already taken"
			auth.AssignForm(form, data)
			r.HTML(200, "user/signup", data)
			return
		}

		log.Error("user.SignUp: %v", err)
		r.HTML(500, "status/500", nil)
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
