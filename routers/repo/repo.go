// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"path"
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/git"
	"github.com/gogits/webdav"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func Create(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = "Create repository"
	ctx.Data["PageIsNewRepo"] = true // For navbar arrow.
	ctx.Data["LanguageIgns"] = models.LanguageIgns
	ctx.Data["Licenses"] = models.Licenses

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "repo/create")
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, "repo/create")
		return
	}

	_, err := models.CreateRepository(ctx.User, form.RepoName, form.Description,
		form.Language, form.License, form.Visibility == "private", form.InitReadme == "on")
	if err == nil {
		log.Trace("%s Repository created: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, form.RepoName)
		ctx.Redirect("/" + ctx.User.Name + "/" + form.RepoName)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.RenderWithErr("Repository name has already been used", "repo/create", &form)
		return
	} else if err == models.ErrRepoNameIllegal {
		ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), "repo/create", &form)
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
			ctx.HTML(200, "repo/setting")
			return
		}

		if err := models.DeleteRepository(ctx.User.Id, ctx.Repo.Repository.Id, ctx.User.LowerName); err != nil {
			ctx.Handle(200, "repo.Delete", err)
			return
		}
	}

	log.Trace("%s Repository deleted: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, ctx.Repo.Repository.LowerName)
	ctx.Redirect("/")
}

func Branches(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsValid {
		return
	}

	brs, err := models.GetBranches(params["username"], params["reponame"])
	if err != nil {
		ctx.Handle(200, "repo.Branches", err)
		return
	} else if len(brs) == 0 {
		ctx.Error(404)
		return
	}

	ctx.Data["Username"] = params["username"]
	ctx.Data["Reponame"] = params["reponame"]

	ctx.Data["Branchname"] = brs[0]
	ctx.Data["Branches"] = brs
	ctx.Data["IsRepoToolbarBranches"] = true

	ctx.HTML(200, "repo/branches")
}

func Single(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsValid {
		return
	}

	if len(params["branchname"]) == 0 {
		params["branchname"] = "master"
	}

	// Get tree path
	treename := params["_1"]

	if len(treename) > 0 && treename[len(treename)-1] == '/' {
		ctx.Redirect("/" + ctx.Repo.Owner.LowerName + "/" +
			ctx.Repo.Repository.Name + "/src/" + params["branchname"] + "/" + treename[:len(treename)-1])
		return
	}

	ctx.Data["IsRepoToolbarSource"] = true

	// Branches.
	brs, err := models.GetBranches(params["username"], params["reponame"])
	if err != nil {
		log.Error("repo.Single(GetBranches): %v", err)
		ctx.Error(404)
		return
	} else if ctx.Repo.Repository.IsBare {
		ctx.Data["IsBareRepo"] = true
		ctx.HTML(200, "repo/single")
		return
	}

	ctx.Data["Branches"] = brs

	repoFile, err := models.GetTargetFile(params["username"], params["reponame"],
		params["branchname"], params["commitid"], treename)

	if err != nil && err != models.ErrRepoFileNotExist {
		log.Error("repo.Single(GetTargetFile): %v", err)
		ctx.Error(404)
		return
	}

	branchLink := "/" + ctx.Repo.Owner.LowerName + "/" + ctx.Repo.Repository.Name + "/src/" + params["branchname"]

	if len(treename) != 0 && repoFile == nil {
		ctx.Error(404)
		return
	}

	if repoFile != nil && repoFile.IsFile() {
		if repoFile.Size > 1024*1024 || repoFile.Filemode != git.FileModeBlob {
			ctx.Data["FileIsLarge"] = true
		} else if blob, err := repoFile.LookupBlob(); err != nil {
			log.Error("repo.Single(repoFile.LookupBlob): %v", err)
			ctx.Error(404)
		} else {
			ctx.Data["IsFile"] = true
			ctx.Data["FileName"] = repoFile.Name
			ext := path.Ext(repoFile.Name)
			if len(ext) > 0 {
				ext = ext[1:]
			}
			ctx.Data["FileExt"] = ext

			readmeExist := base.IsMarkdownFile(repoFile.Name) || base.IsReadmeFile(repoFile.Name)
			ctx.Data["ReadmeExist"] = readmeExist
			if readmeExist {
				ctx.Data["FileContent"] = string(base.RenderMarkdown(blob.Contents(), ""))
			} else {
				ctx.Data["FileContent"] = string(blob.Contents())
			}
		}

	} else {
		// Directory and file list.
		files, err := models.GetReposFiles(params["username"], params["reponame"],
			params["branchname"], params["commitid"], treename)
		if err != nil {
			log.Error("repo.Single(GetReposFiles): %v", err)
			ctx.Error(404)
			return
		}

		ctx.Data["Files"] = files

		var readmeFile *models.RepoFile

		for _, f := range files {
			if !f.IsFile() || !base.IsReadmeFile(f.Name) {
				continue
			} else {
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
				log.Error("repo.Single(readmeFile.LookupBlob): %v", err)
				ctx.Error(404)
				return
			} else {
				// current repo branch link

				ctx.Data["FileName"] = readmeFile.Name
				ctx.Data["FileContent"] = string(base.RenderMarkdown(blob.Contents(), branchLink))
			}
		}
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

		ctx.Data["HasParentPath"] = true
		if len(Paths)-2 >= 0 {
			ctx.Data["ParentPath"] = "/" + Paths[len(Paths)-2]
		}
	}

	// Get latest commit according username and repo name
	commit, err := models.GetCommit(params["username"], params["reponame"],
		params["branchname"], params["commitid"])
	if err != nil {
		log.Error("repo.Single(GetCommit): %v", err)
		ctx.Error(404)
		return
	}
	ctx.Data["LastCommit"] = commit

	ctx.Data["Paths"] = Paths
	ctx.Data["Treenames"] = treenames
	ctx.Data["BranchLink"] = branchLink
	ctx.HTML(200, "repo/single")
}

func Http(ctx *middleware.Context, params martini.Params) {
	/*if !ctx.Repo.IsValid {
		return
	}*/

	// TODO: access check

	username := params["username"]
	reponame := params["reponame"]
	if strings.HasSuffix(reponame, ".git") {
		reponame = reponame[:len(reponame)-4]
	}

	prefix := path.Join("/", username, params["reponame"])
	server := &webdav.Server{
		Fs:         webdav.Dir(models.RepoPath(username, reponame)),
		TrimPrefix: prefix,
		Listings:   true,
	}

	server.ServeHTTP(ctx.ResponseWriter, ctx.Req)
}

func Setting(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsOwner {
		ctx.Error(404)
		return
	}

	ctx.Data["IsRepoToolbarSetting"] = true

	if ctx.Repo.Repository.IsBare {
		ctx.Data["IsBareRepo"] = true
		ctx.HTML(200, "repo/setting")
		return
	}

	var title string
	if t, ok := ctx.Data["Title"].(string); ok {
		title = t
	}

	if len(params["branchname"]) == 0 {
		params["branchname"] = "master"
	}

	ctx.Data["Branchname"] = params["branchname"]
	ctx.Data["Title"] = title + " - settings"
	ctx.HTML(200, "repo/setting")
}

func Commits(ctx *middleware.Context, params martini.Params) {
	brs, err := models.GetBranches(params["username"], params["reponame"])
	if err != nil {
		ctx.Handle(200, "repo.Commits", err)
		return
	} else if len(brs) == 0 {
		ctx.Error(404)
		return
	}

	ctx.Data["IsRepoToolbarCommits"] = true
	commits, err := models.GetCommits(params["username"],
		params["reponame"], params["branchname"])
	if err != nil {
		ctx.Error(404)
		return
	}
	ctx.Data["Username"] = params["username"]
	ctx.Data["Reponame"] = params["reponame"]
	ctx.Data["CommitCount"] = commits.Len()
	ctx.Data["Commits"] = commits
	ctx.HTML(200, "repo/commits")
}

func Pulls(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarPulls"] = true
	ctx.HTML(200, "repo/pulls")
}

func Action(ctx *middleware.Context, params martini.Params) {
	var err error
	switch params["action"] {
	case "watch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, true)
	case "unwatch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, false)
	case "desc":
		if !ctx.Repo.IsOwner {
			ctx.Error(404)
			return
		}

		ctx.Repo.Repository.Description = ctx.Query("desc")
		ctx.Repo.Repository.Website = ctx.Query("site")
		err = models.UpdateRepository(ctx.Repo.Repository)
	}

	if err != nil {
		log.Error("repo.Action(%s): %v", params["action"], err)
		ctx.JSON(200, map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}
	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}
