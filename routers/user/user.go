// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"net/http"

	"github.com/martini-contrib/render"

	//"github.com/gogits/gogs/utils/log"
	"github.com/gogits/gogs/models"
)

func SignIn(r render.Render) {
	r.Redirect("/user/signup", 302)
}

func SignUp(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "user/signup", map[string]interface{}{
			"Title": "Sign Up",
		})
		return
	}

	// TODO: validate form.
	err := models.RegisterUser(&models.User{
		Name:   req.FormValue("username"),
		Email:  req.FormValue("email"),
		Passwd: req.FormValue("passwd"),
	})
	r.HTML(403, "status/403", map[string]interface{}{
		"Title": fmt.Sprintf("%v", err),
	})
}
