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

	// Give default value for template to render.
	ctx.Data["gitignore"] = "0"
	ctx.Data["license"] = "0"
	ctx.Data["Gitignores"] = models.Gitignores
	ctx.Data["Licenses"] = models.Licenses

	ctxUser := ctx.User
	if orgId := com.StrTo(ctx.Query("org")).MustInt64(); orgId > 0 {
		org, err := models.GetUserById(orgId)
		if err != nil && err != models.ErrUserNotExist {
			ctx.Handle(500, "GetUserById", err)
			return
		}
		ctxUser = org
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	ctx.HTML(200, CREATE)
}

func CreatePost(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = ctx.Tr("new_repo")

	ctx.Data["Gitignores"] = models.Gitignores
	ctx.Data["Licenses"] = models.Licenses

	ctxUser := ctx.User
	// Not equal means current user is an organization.
	if form.Uid != ctx.User.Id {
		org, err := models.GetUserById(form.Uid)
		if err != nil && err != models.ErrUserNotExist {
			ctx.Handle(500, "GetUserById", err)
			return
		}
		ctxUser = org
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	if ctx.HasError() {
		ctx.HTML(200, CREATE)
		return
	}

	if ctxUser.IsOrganization() {
		// Check ownership of organization.
		if !ctxUser.IsOrgOwner(ctx.User.Id) {
			ctx.Error(403)
			return
		}
	}

	repo, err := models.CreateRepository(ctxUser, form.RepoName, form.Description,
		form.Gitignore, form.License, form.Private, false, form.InitReadme)
	if err == nil {
		log.Trace("Repository created: %s/%s", ctxUser.Name, form.RepoName)
		ctx.Redirect("/" + ctxUser.Name + "/" + form.RepoName)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), CREATE, &form)
		return
	} else if err == models.ErrRepoNameIllegal {
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("form.illegal_repo_name"), CREATE, &form)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.Id, repo.Id, ctxUser.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}
	ctx.Handle(500, "CreatePost", err)
}

func Migrate(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_migrate")

	ctxUser := ctx.User
	if orgId := com.StrTo(ctx.Query("org")).MustInt64(); orgId > 0 {
		org, err := models.GetUserById(orgId)
		if err != nil && err != models.ErrUserNotExist {
			ctx.Handle(500, "GetUserById", err)
			return
		}
		ctxUser = org
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	ctx.HTML(200, MIGRATE)
}

func MigratePost(ctx *middleware.Context, form auth.MigrateRepoForm) {
	ctx.Data["Title"] = ctx.Tr("new_migrate")

	ctxUser := ctx.User
	if orgId := com.StrTo(ctx.Query("org")).MustInt64(); orgId > 0 {
		org, err := models.GetUserById(orgId)
		if err != nil && err != models.ErrUserNotExist {
			ctx.Handle(500, "GetUserById", err)
			return
		}
		ctxUser = org
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	if ctx.HasError() {
		ctx.HTML(200, MIGRATE)
		return
	}

	if ctxUser.IsOrganization() {
		// Check ownership of organization.
		if !ctxUser.IsOrgOwner(ctx.User.Id) {
			ctx.Error(403)
			return
		}
	}

	authStr := strings.Replace(fmt.Sprintf("://%s:%s",
		form.AuthUserName, form.AuthPasswd), "@", "%40", -1)
	url := strings.Replace(form.HttpsUrl, "://", authStr+"@", 1)
	repo, err := models.MigrateRepository(ctxUser, form.RepoName, form.Description, form.Private,
		form.Mirror, url)
	if err == nil {
		log.Trace("Repository migrated: %s/%s", ctxUser.Name, form.RepoName)
		ctx.Redirect("/" + ctxUser.Name + "/" + form.RepoName)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), MIGRATE, &form)
		return
	} else if err == models.ErrRepoNameIllegal {
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("form.illegal_repo_name"), MIGRATE, &form)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.Id, repo.Id, ctxUser.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	if strings.Contains(err.Error(), "Authentication failed") {
		ctx.Data["Err_Auth"] = true
		ctx.RenderWithErr(ctx.Tr("form.auth_failed", err), MIGRATE, &form)
		return
	}
	ctx.Handle(500, "MigratePost", err)
}

func Action(ctx *middleware.Context) {
	var err error
	switch ctx.Params(":action") {
	case "watch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, true)
	case "unwatch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, false)
	case "star":
		err = models.StarRepo(ctx.User.Id, ctx.Repo.Repository.Id, true)
	case "unstar":
		err = models.StarRepo(ctx.User.Id, ctx.Repo.Repository.Id, false)
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
		log.Error(4, "repo.Action(%s): %v", ctx.Params(":action"), err)
		ctx.JSON(200, map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink)
	return
	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

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
