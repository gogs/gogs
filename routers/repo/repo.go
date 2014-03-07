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
)

func Create(req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	data["Title"] = "Create repository"

	if req.Method == "GET" {
		r.HTML(200, "repo/create", map[string]interface{}{
			"UserName": auth.SignedInName(session),
			"UserId":   auth.SignedInId(session),
			"IsSigned": auth.IsSignedIn(session),
		})
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
			r.HTML(200, "repo/created", map[string]interface{}{
				"RepoName": u.Name + "/" + req.FormValue("name"),
				"IsSigned": auth.IsSignedIn(session),
			})
			return
		}
	}

	if err != nil {
		r.HTML(200, "base/error", map[string]interface{}{
			"Error":    fmt.Sprintf("%v", err),
			"IsSigned": auth.IsSignedIn(session),
		})
	}
}

func Delete(req *http.Request, r render.Render, session sessions.Session) {
	if req.Method == "GET" {
		r.HTML(200, "repo/delete", map[string]interface{}{
			"Title":    "Delete repository",
			"IsSigned": auth.IsSignedIn(session),
		})
		return
	}

	u := &models.User{}
	err := models.DeleteRepository(u, "")
	if err != nil {
		r.HTML(200, "base/error", map[string]interface{}{
			"Error":    fmt.Sprintf("%v", err),
			"IsSigned": auth.IsSignedIn(session),
		})
	}
}

func List(req *http.Request, r render.Render, session sessions.Session) {
	u := auth.SignedInUser(session)
	repos, err := models.GetRepositories(u)
	fmt.Println("repos", repos)
	if err != nil {
		r.HTML(200, "base/error", map[string]interface{}{
			"Error":    fmt.Sprintf("%v", err),
			"IsSigned": auth.IsSignedIn(session),
		})
		return
	}

	r.HTML(200, "repo/list", map[string]interface{}{
		"Title":    "repositories",
		"Repos":    repos,
		"IsSigned": auth.IsSignedIn(session),
	})
}
