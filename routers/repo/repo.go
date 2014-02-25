// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/martini-contrib/render"

	"github.com/gogits/gogs/models"
)

func Create(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "repo/create", map[string]interface{}{
			"Title": "Create repository",
		})
		return
	}

	// TODO: access check
	fmt.Println(req.FormValue("userId"), req.FormValue("name"))

	id, err := strconv.ParseInt(req.FormValue("userId"), 10, 64)
	if err == nil {
		var user *models.User
		user, err = models.GetUserById(id)
		if user == nil {
			err = models.ErrUserNotExist
		}
		if err == nil {
			_, err = models.CreateRepository(user, req.FormValue("name"))
		}
		if err == nil {
			r.HTML(200, "repo/created", map[string]interface{}{
				"RepoName": user.Name + "/" + req.FormValue("name"),
			})
			return
		}
	}

	if err != nil {
		r.HTML(403, "status/403", map[string]interface{}{
			"Title": fmt.Sprintf("%v", err),
		})
	}
}

func Delete(req *http.Request, r render.Render) {
	if req.Method == "GET" {
		r.HTML(200, "repo/delete", map[string]interface{}{
			"Title": "Delete repository",
		})
		return
	}

	u := &models.User{}
	err := models.DeleteRepository(u, "")
	r.HTML(403, "status/403", map[string]interface{}{
		"Title": fmt.Sprintf("%v", err),
	})
}
