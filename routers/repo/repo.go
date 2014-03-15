// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/middleware"
)

func Create(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = "Create repository"

	if ctx.Req.Method == "GET" {
		ctx.Data["LanguageIgns"] = models.LanguageIgns
		ctx.Data["Licenses"] = models.Licenses
		ctx.Render.HTML(200, "repo/create", ctx.Data)
		return
	}

	if ctx.HasError() {
		ctx.Render.HTML(200, "repo/create", ctx.Data)
		return
	}

	// TODO: access check

	user, err := models.GetUserById(form.UserId)
	if err != nil {
		if err.Error() == models.ErrUserNotExist.Error() {
			ctx.RenderWithErr("User does not exist", "repo/create", &form)
			return
		}
	}

	if err == nil {
		if _, err = models.CreateRepository(user,
			form.RepoName, form.Description, form.Language, form.License,
			form.Visibility == "private", form.InitReadme == "on"); err == nil {
			ctx.Render.Redirect("/"+user.Name+"/"+form.RepoName, 302)
			return
		}
	}

	if err.Error() == models.ErrRepoAlreadyExist.Error() {
		ctx.RenderWithErr("Repository name has already been used", "repo/create", &form)
		return
	}

	ctx.Handle(200, "repo.Create", err)
}

func Delete(ctx *middleware.Context, form auth.DeleteRepoForm) {
	ctx.Data["Title"] = "Delete repository"

	if ctx.Req.Method == "GET" {
		ctx.Render.HTML(200, "repo/delete", ctx.Data)
		return
	}

	if err := models.DeleteRepository(form.UserId, form.RepoId, form.UserName); err != nil {
		ctx.Handle(200, "repo.Delete", err)
		return
	}

	ctx.Render.Redirect("/", 302)
}

func List(ctx *middleware.Context) {
	if ctx.User != nil {
		ctx.Render.Redirect("/")
		return
	}

	ctx.Data["Title"] = "Repositories"
	repos, err := models.GetRepositories(ctx.User)
	if err != nil {
		ctx.Handle(200, "repo.List", err)
		return
	}

	ctx.Data["Repos"] = repos
	ctx.Render.HTML(200, "repo/list", ctx.Data)
}
