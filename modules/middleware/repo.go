// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"errors"
	"fmt"
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
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
					ctx.Redirect("/")
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
				ctx.Redirect("/")
				return
			}
			ctx.Handle(200, "RepoAssignment", errors.New("invliad user account for single repository"))
			return
		}

		ctx.Repo.Owner = user

		// get repository
		repo, err := models.GetRepositoryByName(user.Id, params["reponame"])
		if err != nil {
			if err == models.ErrRepoNotExist {
				ctx.Handle(404, "RepoAssignment", err)
			} else if redirect {
				ctx.Redirect("/")
				return
			}
			ctx.Handle(200, "RepoAssignment", err)
			return
		}

		ctx.Repo.IsValid = true
		if ctx.User != nil {
			ctx.Repo.IsWatching = models.IsWatching(ctx.User.Id, repo.Id)
		}
		ctx.Repo.Repository = repo
		scheme := "http"
		if base.EnableHttpsClone {
			scheme = "https"
		}
		ctx.Repo.CloneLink.SSH = fmt.Sprintf("%s@%s:%s/%s.git", base.RunUser, base.Domain, user.LowerName, repo.LowerName)
		ctx.Repo.CloneLink.HTTPS = fmt.Sprintf("%s://%s/%s/%s.git", scheme, base.Domain, user.LowerName, repo.LowerName)

		if len(params["branchname"]) == 0 {
			params["branchname"] = "master"
		}
		ctx.Data["Branchname"] = params["branchname"]

		ctx.Data["IsRepositoryValid"] = true
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = user
		ctx.Data["Title"] = user.Name + "/" + repo.Name
		ctx.Data["CloneLink"] = ctx.Repo.CloneLink
		ctx.Data["RepositoryLink"] = ctx.Data["Title"]
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner
		ctx.Data["IsRepositoryWatching"] = ctx.Repo.IsWatching
	}
}
