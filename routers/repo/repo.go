// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func Create(req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Create repository"

	if req.Method == "GET" {
		r.HTML(200, "repo/create", data)
		return
	}

	// TODO: access check

	id, err := strconv.ParseInt(req.FormValue("userId"), 10, 64)
	if err == nil {
		var u *models.User
		u, err = models.GetUserById(id)
		if u == nil {
			err = models.ErrUserNotExist
		}
		if err == nil {
			_, err = models.CreateRepository(u, req.FormValue("name"))
		}
		if err == nil {
			data["RepoName"] = u.Name + "/" + req.FormValue("name")
			r.HTML(200, "repo/created", data)
			return
		}
	}

	if err != nil {
		data["ErrorMsg"] = err
		log.Error("repo.Create: %v", err)
		r.HTML(200, "base/error", data)
	}
}

func Delete(req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Delete repository"

	if req.Method == "GET" {
		r.HTML(200, "repo/delete", data)
		return
	}

	u := &models.User{}
	err := models.DeleteRepository(u, "")
	if err != nil {
		data["ErrorMsg"] = err
		log.Error("repo.Delete: %v", err)
		r.HTML(200, "base/error", data)
	}
}

func List(req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Repositories"

	u := auth.SignedInUser(session)
	repos, err := models.GetRepositories(u)
	fmt.Println("repos", repos)
	if err != nil {
		data["ErrorMsg"] = err
		log.Error("repo.List: %v", err)
		r.HTML(200, "base/error", data)
		return
	}

	data["Repos"] = repos
	r.HTML(200, "repo/list", data)
}
