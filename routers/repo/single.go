// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

func Single(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsValid {
		return
	}

	if params["branchname"] == "" {
		params["branchname"] = "master"
	}

	treename := params["_1"]
	files, err := models.GetReposFiles(params["username"], params["reponame"],
		params["branchname"], treename)
	if err != nil {
		ctx.Handle(200, "repo.Single", err)
		return
	}
	ctx.Data["Username"] = params["username"]
	ctx.Data["Reponame"] = params["reponame"]
	ctx.Data["Branchname"] = params["branchname"]

	var treenames []string
	Paths := make([]string, 0)

	if len(treename) > 0 {
		treenames = strings.Split(treename, "/")
		for i, _ := range treenames {
			Paths = append(Paths, strings.Join(treenames[0:i+1], "/"))
		}
	}

	ctx.Data["Paths"] = Paths
	ctx.Data["Treenames"] = treenames
	ctx.Data["IsRepoToolbarSource"] = true
	ctx.Data["Files"] = files
	ctx.Render.HTML(200, "repo/single", ctx.Data)
}

func Setting(ctx *middleware.Context) {
	if !ctx.Repo.IsValid {
		return
	}

	var title string
	if t, ok := ctx.Data["Title"].(string); ok {
		title = t
	}

	ctx.Data["Title"] = title + " - settings"
	ctx.Data["IsRepoToolbarSetting"] = true
	ctx.Render.HTML(200, "repo/setting", ctx.Data)
}

func Commits(ctx *middleware.Context) string {
	return "This is commits page"
}

func Issues(ctx *middleware.Context) string {
	return "This is issues page"
}

func Pulls(ctx *middleware.Context) string {
	return "This is pulls page"
}
