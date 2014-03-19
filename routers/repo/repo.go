// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func Create(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = "Create repository"

	if ctx.Req.Method == "GET" {
		ctx.Data["LanguageIgns"] = models.LanguageIgns
		ctx.Data["Licenses"] = models.Licenses
		ctx.HTML(200, "repo/create", ctx.Data)
		return
	}

	_, err := models.CreateRepository(ctx.User, form.RepoName, form.Description,
		form.Language, form.License, form.Visibility == "private", form.InitReadme == "on")
	if err == nil {
		log.Trace("%s Repository created: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, form.RepoName)
		ctx.Redirect("/"+ctx.User.Name+"/"+form.RepoName, 302)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.RenderWithErr("Repository name has already been used", "repo/create", &form)
		return
	}
	ctx.Handle(200, "repo.Create", err)
}

func SettingPost(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(404)
		return
	}

	switch ctx.Query("action") {
	case "delete":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.Data["ErrorMsg"] = "Please make sure you entered repository name is correct."
			ctx.HTML(200, "repo/setting", ctx.Data)
			return
		}

		if err := models.DeleteRepository(ctx.User.Id, ctx.Repo.Repository.Id, ctx.User.LowerName); err != nil {
			ctx.Handle(200, "repo.Delete", err)
			return
		}
	}

	log.Trace("%s Repository deleted: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, ctx.Repo.Repository.LowerName)
	ctx.Redirect("/", 302)
}
