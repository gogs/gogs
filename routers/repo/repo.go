// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"

	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func Create(form auth.CreateRepoForm, req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Create repository"

	if req.Method == "GET" {
		r.HTML(200, "repo/create", data)
		return
	}

	if hasErr, ok := data["HasError"]; ok && hasErr.(bool) {
		r.HTML(200, "repo/create", data)
		return
	}

	// TODO: access check

	user, err := models.GetUserById(form.UserId)
	if err != nil {
		if err.Error() == models.ErrUserNotExist.Error() {
			data["HasError"] = true
			data["ErrorMsg"] = "User does not exist"
			auth.AssignForm(form, data)
			r.HTML(200, "repo/create", data)
			return
		}
	}
	fmt.Println(models.RepoPath(user.Name, form.RepoName))
	if err == nil {
		if _, err = models.CreateRepository(user,
			form.RepoName, form.Description, form.Visibility == "private"); err == nil {
			// Initialize README.
			if form.InitReadme == "true" {
				// TODO
			}
			// TODO: init .gitignore file
			data["RepoName"] = user.Name + "/" + form.RepoName
			r.HTML(200, "repo/created", data)
			return
		}
	}

	if err.Error() == models.ErrRepoAlreadyExist.Error() {
		data["HasError"] = true
		data["ErrorMsg"] = "Repository name has already been used"
		auth.AssignForm(form, data)
		r.HTML(200, "repo/create", data)
		return
	}

	data["ErrorMsg"] = err
	log.Error("repo.Create: %v", err)
	r.HTML(200, "base/error", data)
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
