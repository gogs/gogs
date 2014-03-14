// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
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
		data["LanguageIgns"] = models.LanguageIgns
		data["Licenses"] = models.Licenses
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

	if err == nil {
		if _, err = models.CreateRepository(user,
			form.RepoName, form.Description, form.Language, form.License,
			form.Visibility == "private", form.InitReadme == "on"); err == nil {
			if err == nil {
				data["RepoName"] = user.Name + "/" + form.RepoName
				r.HTML(200, "repo/created", data)
				return
			}
		}
	}

	if err.Error() == models.ErrRepoAlreadyExist.Error() {
		data["HasError"] = true
		data["ErrorMsg"] = "Repository name has already been used"
		auth.AssignForm(form, data)
		r.HTML(200, "repo/create", data)
		return
	}

	log.Handle(200, "repo.Create", data, r, err)
}

func Delete(form auth.DeleteRepoForm, req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Delete repository"

	if req.Method == "GET" {
		r.HTML(200, "repo/delete", data)
		return
	}

	if err := models.DeleteRepository(form.UserId, form.RepoId, form.UserName); err != nil {
		log.Handle(200, "repo.Delete", data, r, err)
		return
	}

	r.Redirect("/", 302)
}

func List(req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	u := auth.SignedInUser(session)
	if u != nil {
		r.Redirect("/")
		return
	}

	data["Title"] = "Repositories"
	repos, err := models.GetRepositories(u)
	if err != nil {
		log.Handle(200, "repo.List", data, r, err)
		return
	}

	data["Repos"] = repos
	r.HTML(200, "repo/list", data)
}
