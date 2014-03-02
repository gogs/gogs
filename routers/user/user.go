// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/http"

	"github.com/martini-contrib/render"

	"github.com/gogits/validation"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/utils/log"
)

func Profile(r render.Render) {
	r.HTML(200, "user/profile", map[string]interface{}{
		"Title": "Username",
	})
	return
}

func SignIn(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "user/signin", map[string]interface{}{
			"Title": "Log In",
		})
		return
	}

	// todo sign in
	_, err := models.LoginUserPlain(req.FormValue("account"), req.FormValue("passwd"))
	if err != nil {
		r.HTML(200, "base/error", map[string]interface{}{
			"Error": fmt.Sprintf("%v", err),
		})
		return
	}
	r.Redirect("/")
}

func SignUp(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "user/signup", map[string]interface{}{
			"Title": "Sign Up",
		})
		return
	}

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
		for _, err := range valid.Errors {
			log.Warn("user.SignUp -> valid user: %v", err)
		}
		return
	}

	err = models.RegisterUser(u)
	if err != nil {
		if err != nil {
			r.HTML(200, "base/error", map[string]interface{}{
				"Error": fmt.Sprintf("%v", err),
			})
			return
		}
	}

	r.Redirect("/")
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
