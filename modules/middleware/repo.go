// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"errors"
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
)

func RepoAssignment(redirect bool) martini.Handler {
	return func(ctx *Context, params martini.Params) {
		// assign false first
		ctx.Data["IsRepositoryValid"] = false

		var (
			user *models.User
			err  error
		)

		// get repository owner
		ctx.Repo.IsOwner = ctx.IsSigned && ctx.User.LowerName == strings.ToLower(params["username"])

		if !ctx.Repo.IsOwner {
			user, err = models.GetUserByName(params["username"])
			if err != nil {
				if redirect {
					ctx.Render.Redirect("/")
					return
				}
				ctx.Handle(200, "RepoAssignment", err)
				return
			}
		} else {
			user = ctx.User
		}

		if user == nil {
			if redirect {
				ctx.Render.Redirect("/")
				return
			}
			ctx.Handle(200, "RepoAssignment", errors.New("invliad user account for single repository"))
			return
		}

		ctx.Repo.Owner = user

		// get repository
		repo, err := models.GetRepositoryByName(user, params["reponame"])
		if err != nil {
			if redirect {
				ctx.Render.Redirect("/")
				return
			}
			ctx.Handle(200, "RepoAssignment", err)
			return
		}

		ctx.Repo.IsValid = true
		ctx.Repo.Repository = repo

		ctx.Data["IsRepositoryValid"] = true
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = user
		ctx.Data["Title"] = user.Name + "/" + repo.Name
		ctx.Data["RepositoryLink"] = ctx.Data["Title"]
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner
	}
}
