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

func Profile(r render.Render) {
	r.HTML(200, "user/profile", map[string]interface{}{
		"Title": "Username",
	})
	return
}

func SignIn(req *http.Request, r render.Render, session sessions.Session) {
	var (
		errString string
		account   string
	)
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
