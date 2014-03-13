// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"reflect"

	"github.com/codegangsta/martini"

	"github.com/gogits/binding"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
)

type CreateRepoForm struct {
	UserId      int64  `form:"userId"`
	RepoName    string `form:"repo" binding:"Required;AlphaDash"`
	Visibility  string `form:"visibility"`
	Description string `form:"desc" binding:"MaxSize(100)"`
	Language    string `form:"language"`
	License     string `form:"license"`
	InitReadme  string `form:"initReadme"`
}

func (f *CreateRepoForm) Name(field string) string {
	names := map[string]string{
		"RepoName":    "Repository name",
		"Description": "Description",
	}
	return names[field]
}

func (f *CreateRepoForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true
	AssignForm(f, data)

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("CreateRepoForm.Validate: %v", err)
		}
		return
	}

	validate(errors, data, f)
}

func RepoAssignment(redirect bool) martini.Handler {
	return func(params martini.Params, r render.Render, data base.TmplData, session sessions.Session) {
		// assign false first
		data["IsRepositoryValid"] = false

		var (
			user *models.User
			err  error
		)
		// get repository owner
		isOwner := (data["SignedUserName"] == params["username"])
		if !isOwner {
			user, err = models.GetUserByName(params["username"])
			if err != nil {
				if redirect {
					r.Redirect("/")
					return
				}
				//data["ErrorMsg"] = err
				//log.Error("repo.Single: %v", err)
				//r.HTML(200, "base/error", data)
				return
			}
		} else {
			user = SignedInUser(session)
		}
		if user == nil {
			if redirect {
				r.Redirect("/")
				return
			}
			//data["ErrorMsg"] = "invliad user account for single repository"
			//log.Error("repo.Single: %v", err)
			//r.HTML(200, "base/error", data)
			return
		}
		data["IsRepositoryOwner"] = isOwner

		// get repository
		repo, err := models.GetRepositoryByName(user, params["reponame"])
		if err != nil {
			if redirect {
				r.Redirect("/")
				return
			}
			//data["ErrorMsg"] = err
			//log.Error("repo.Single: %v", err)
			//r.HTML(200, "base/error", data)
			return
		}

		data["Repository"] = repo
		data["Owner"] = user
		data["Title"] = user.Name + "/" + repo.Name
		data["RepositoryLink"] = data["Title"]
		data["IsRepositoryValid"] = true
	}
}
