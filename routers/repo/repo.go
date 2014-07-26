// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	CREATE  base.TplName = "repo/create"
	MIGRATE base.TplName = "repo/migrate"
)

func Create(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_repo")
	ctx.Data["PageIsRepoCreate"] = true

	// Give default value for template to render.
	ctx.Data["gitignore"] = "0"
	ctx.Data["license"] = "0"
	ctx.Data["Gitignores"] = models.Gitignores
	ctx.Data["Licenses"] = models.Licenses

	ctxUser := ctx.User
	orgId := com.StrTo(ctx.Query("org")).MustInt64()
	if orgId > 0 {
		org, err := models.GetUserById(orgId)
		if err != nil && err != models.ErrUserNotExist {
			ctx.Handle(500, "home.Dashboard(GetUserById)", err)
			return
		}
		ctxUser = org
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "home.Dashboard(GetOrganizations)", err)
		return
	}
	ctx.Data["AllUsers"] = append([]*models.User{ctx.User}, ctx.User.Orgs...)

	ctx.HTML(200, CREATE)
}

func CreatePost(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = ctx.Tr("new_repo")
	ctx.Data["PageIsRepoCreate"] = true

	ctx.Data["Gitignores"] = models.Gitignores
	ctx.Data["Licenses"] = models.Licenses

	ctxUser := ctx.User
	orgId := com.StrTo(ctx.Query("org")).MustInt64()
	if orgId > 0 {
		org, err := models.GetUserById(orgId)
		if err != nil && err != models.ErrUserNotExist {
			ctx.Handle(500, "home.Dashboard(GetUserById)", err)
			return
		}
		ctxUser = org
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "home.CreatePost(GetOrganizations)", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	if ctx.HasError() {
		ctx.HTML(200, CREATE)
		return
	}

	u := ctx.User
	// Not equal means current user is an organization.
	if u.Id != form.Uid {
		var err error
		u, err = models.GetUserById(form.Uid)
		if err != nil {
			if err == models.ErrUserNotExist {
				ctx.Handle(404, "home.CreatePost(GetUserById)", err)
			} else {
				ctx.Handle(500, "home.CreatePost(GetUserById)", err)
			}
			return
		}

		// Check ownership of organization.
		if !u.IsOrgOwner(ctx.User.Id) {
			ctx.Error(403)
			return
		}
	}

	repo, err := models.CreateRepository(u, form.RepoName, form.Description,
		form.Gitignore, form.License, form.Private, false, form.InitReadme)
	if err == nil {
		log.Trace("Repository created: %s/%s", u.Name, form.RepoName)
		ctx.Redirect("/" + u.Name + "/" + form.RepoName)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), CREATE, &form)
		return
	} else if err == models.ErrRepoNameIllegal {
		ctx.RenderWithErr(ctx.Tr("form.illegal_repo_name"), CREATE, &form)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(u.Id, repo.Id, u.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}
	ctx.Handle(500, "CreateRepository", err)
}

func Migrate(ctx *middleware.Context) {
	ctx.Data["Title"] = "Migrate repository"
	ctx.Data["PageIsNewRepo"] = true

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "home.Migrate(GetOrganizations)", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	ctx.HTML(200, MIGRATE)
}

func MigratePost(ctx *middleware.Context, form auth.MigrateRepoForm) {
	ctx.Data["Title"] = "Migrate repository"
	ctx.Data["PageIsNewRepo"] = true

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "home.MigratePost(GetOrganizations)", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	if ctx.HasError() {
		ctx.HTML(200, MIGRATE)
		return
	}

	u := ctx.User
	// Not equal means current user is an organization.
	if u.Id != form.Uid {
		var err error
		u, err = models.GetUserById(form.Uid)
		if err != nil {
			if err == models.ErrUserNotExist {
				ctx.Handle(404, "home.MigratePost(GetUserById)", err)
			} else {
				ctx.Handle(500, "home.MigratePost(GetUserById)", err)
			}
			return
		}
	}

	authStr := strings.Replace(fmt.Sprintf("://%s:%s",
		form.AuthUserName, form.AuthPasswd), "@", "%40", -1)
	url := strings.Replace(form.Url, "://", authStr+"@", 1)
	repo, err := models.MigrateRepository(u, form.RepoName, form.Description, form.Private,
		form.Mirror, url)
	if err == nil {
		log.Trace("%s Repository migrated: %s/%s", ctx.Req.RequestURI, u.LowerName, form.RepoName)
		ctx.Redirect("/" + u.Name + "/" + form.RepoName)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.RenderWithErr("Repository name has already been used", MIGRATE, &form)
		return
	} else if err == models.ErrRepoNameIllegal {
		ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), MIGRATE, &form)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(u.Id, repo.Id, u.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	if strings.Contains(err.Error(), "Authentication failed") {
		ctx.RenderWithErr(err.Error(), MIGRATE, &form)
		return
	}
	ctx.Handle(500, "MigrateRepository", err)
}

// func Action(ctx *middleware.Context, params martini.Params) {
// 	var err error
// 	switch params["action"] {
// 	case "watch":
// 		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, true)
// 	case "unwatch":
// 		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, false)
// 	case "desc":
// 		if !ctx.Repo.IsOwner {
// 			ctx.Error(404)
// 			return
// 		}

// 		ctx.Repo.Repository.Description = ctx.Query("desc")
// 		ctx.Repo.Repository.Website = ctx.Query("site")
// 		err = models.UpdateRepository(ctx.Repo.Repository)
// 	}

// 	if err != nil {
// 		log.Error("repo.Action(%s): %v", params["action"], err)
// 		ctx.JSON(200, map[string]interface{}{
// 			"ok":  false,
// 			"err": err.Error(),
// 		})
// 		return
// 	}
// 	ctx.JSON(200, map[string]interface{}{
// 		"ok": true,
// 	})
// }

func Download(ctx *middleware.Context) {
	ext := "." + ctx.Params(":ext")

	var archivePath string
	switch ext {
	case ".zip":
		archivePath = path.Join(ctx.Repo.GitRepo.Path, "archives/zip")
	case ".tar.gz":
		archivePath = path.Join(ctx.Repo.GitRepo.Path, "archives/targz")
	default:
		ctx.Error(404)
		return
	}

	if !com.IsDir(archivePath) {
		if err := os.MkdirAll(archivePath, os.ModePerm); err != nil {
			ctx.Handle(500, "Download -> os.MkdirAll(archivePath)", err)
			return
		}
	}

	archivePath = path.Join(archivePath, ctx.Repo.CommitId+ext)
	if !com.IsFile(archivePath) {
		if err := ctx.Repo.Commit.CreateArchive(archivePath, git.ZIP); err != nil {
			ctx.Handle(500, "Download -> CreateArchive "+archivePath, err)
			return
		}
	}

	ctx.ServeFile(archivePath, ctx.Repo.Repository.Name+"-"+base.ShortSha(ctx.Repo.CommitId)+ext)
}
