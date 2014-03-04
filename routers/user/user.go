// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/http"

	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/validation"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/utils/auth"
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

func SignUp(req *http.Request, r render.Render) {
	data := map[string]interface{}{"Title": "Sign Up"}
	if req.Method == "GET" {
		r.HTML(200, "user/signup", data)
		return
	}

	// Front-end should do double check of password.
	u := &models.User{
		Name:   req.FormValue("username"),
		Email:  req.FormValue("email"),
		Passwd: req.FormValue("passwd"),
	}

	valid := validation.Validation{}
	ok, err := valid.Valid(u)
	if err != nil {
		log.Error("user.SignUp -> valid user: %v", err)
		return
	}
	if !ok {
		data["HasError"] = true
		data["ErrorMsg"] = auth.GenerateErrorMsg(valid.Errors[0])
		r.HTML(200, "user/signup", data)
		return
	}

	// err = models.RegisterUser(u)
	// if err != nil {
	// 	r.HTML(200, "base/error", map[string]interface{}{
	// 		"Error": fmt.Sprintf("%v", err),
	// 	})
	// 	return
	// }

	// r.Redirect("/")
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
