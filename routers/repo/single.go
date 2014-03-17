// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/git"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Branches(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsValid {
		return
	}

	ctx.Data["Username"] = params["username"]
	ctx.Data["Reponame"] = params["reponame"]

	brs, err := models.GetBranches(params["username"], params["reponame"])
	if err != nil {
		ctx.Handle(200, "repo.Branches", err)
		return
	}

	ctx.Data["Branchname"] = brs[0]
	ctx.Data["Branches"] = brs
	ctx.Data["IsRepoToolbarBranches"] = true

	ctx.Render.HTML(200, "repo/branches", ctx.Data)
}

func Single(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsValid {
		return
	}

	if params["branchname"] == "" {
		params["branchname"] = "master"
	}

	// Get tree path
	treename := params["_1"]

	// Directory and file list.
	files, err := models.GetReposFiles(params["username"], params["reponame"],
		params["branchname"], treename)
	if err != nil {
		ctx.Handle(200, "repo.Single(GetReposFiles)", err)
		return
	}
	ctx.Data["Username"] = params["username"]
	ctx.Data["Reponame"] = params["reponame"]
	ctx.Data["Branchname"] = params["branchname"]

	// Branches.
	brs, err := models.GetBranches(params["username"], params["reponame"])
	if err != nil {
		ctx.Handle(200, "repo.Single(GetBranches)", err)
		return
	}
	ctx.Data["Branches"] = brs

	var treenames []string
	Paths := make([]string, 0)

	if len(treename) > 0 {
		treenames = strings.Split(treename, "/")
		for i, _ := range treenames {
			Paths = append(Paths, strings.Join(treenames[0:i+1], "/"))
		}
	}

	// Get latest commit according username and repo name
	commit, err := models.GetLastestCommit(params["username"], params["reponame"])
	if err != nil {
		ctx.Handle(200, "repo.Single(GetLastestCommit)", err)
		return
	}
	ctx.Data["LatestCommit"] = commit

	var readmeFile *models.RepoFile

	for _, f := range files {
		if !f.IsFile() || len(f.Name) < 6 {
			continue
		} else if strings.ToLower(f.Name[:6]) == "readme" {
			readmeFile = f
			break
		}
	}

	if readmeFile != nil {
		ctx.Data["ReadmeExist"] = true
		// if file large than 1M not show it
		if readmeFile.Size > 1024*1024 || readmeFile.Filemode != git.FileModeBlob {
			ctx.Data["FileIsLarge"] = true
		} else if blob, err := readmeFile.LookupBlob(); err != nil {
			ctx.Data["ReadmeExist"] = false
		} else {
			// current repo branch link
			urlPrefix := "http://" + base.Domain + "/" + ctx.Repo.Owner.LowerName + "/" +
				ctx.Repo.Repository.Name + "/blob/" + params["branchname"]

			ctx.Data["ReadmeContent"] = string(base.RenderMarkdown(blob.Contents(), urlPrefix))
		}
	}

	ctx.Data["Paths"] = Paths
	ctx.Data["Treenames"] = treenames
	ctx.Data["IsRepoToolbarSource"] = true
	ctx.Data["Files"] = files
	ctx.Render.HTML(200, "repo/single", ctx.Data)
}

func Setting(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsOwner {
		ctx.Render.Error(404)
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

func Commits(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsRepoToolbarCommits"] = true
	commits, err := models.GetCommits(params["username"],
		params["reponame"], params["branchname"])
	if err != nil {
		ctx.Render.Error(404)
		return
	}
	ctx.Data["Commits"] = commits
	ctx.Render.HTML(200, "repo/commits", ctx.Data)
}

func Issues(ctx *middleware.Context) string {
	return "This is issues page"
}

func Pulls(ctx *middleware.Context) string {
	return "This is pulls page"
}
