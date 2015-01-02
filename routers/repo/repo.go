// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/url"
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
	"github.com/gogits/gogs/modules/setting"
)

const (
	CREATE  base.TplName = "repo/create"
	MIGRATE base.TplName = "repo/migrate"
	FORK    base.TplName = "repo/fork"
)

func checkContextUser(ctx *middleware.Context, uid int64) (*models.User, error) {
	ctxUser := ctx.User
	if uid > 0 {
		org, err := models.GetUserById(uid)
		if err != models.ErrUserNotExist {
			if err != nil {
				return nil, fmt.Errorf("GetUserById: %v", err)
			}
			ctxUser = org
		}
	}
	return ctxUser, nil
}

func Create(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_repo")

	// Give default value for template to render.
	ctx.Data["gitignore"] = "0"
	ctx.Data["license"] = "0"
	ctx.Data["Gitignores"] = models.Gitignores
	ctx.Data["Licenses"] = models.Licenses

	ctxUser, err := checkContextUser(ctx, ctx.QueryInt64("org"))
	if err != nil {
		ctx.Handle(500, "checkContextUser", err)
		return
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
		var err error
		ctxUser, err = checkContextUser(ctx, form.Uid)
		if err != nil {
			ctx.Handle(500, "checkContextUser", err)
			return
		}
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
		if !ctxUser.IsOwnedBy(ctx.User.Id) {
			ctx.Error(403)
			return
		}
	}

	repo, err := models.CreateRepository(ctxUser, form.RepoName, form.Description,
		form.Gitignore, form.License, form.Private, false, form.AutoInit)
	if err == nil {
		log.Trace("Repository created: %s/%s", ctxUser.Name, repo.Name)
		ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + repo.Name)
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

	ctxUser, err := checkContextUser(ctx, ctx.QueryInt64("org"))
	if err != nil {
		ctx.Handle(500, "checkContextUser", err)
		return
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
	// Not equal means current user is an organization.
	if form.Uid != ctx.User.Id {
		var err error
		ctxUser, err = checkContextUser(ctx, form.Uid)
		if err != nil {
			ctx.Handle(500, "checkContextUser", err)
			return
		}
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
		if !ctxUser.IsOwnedBy(ctx.User.Id) {
			ctx.Error(403)
			return
		}
	}

	u, err := url.Parse(form.HttpsUrl)

	if err != nil || u.Scheme != "https" {
		ctx.Data["Err_HttpsUrl"] = true
		ctx.RenderWithErr(ctx.Tr("form.url_error"), MIGRATE, &form)
		return
	}

	if len(form.AuthUserName) > 0 || len(form.AuthPasswd) > 0 {
		u.User = url.UserPassword(form.AuthUserName, form.AuthPasswd)
	}

	repo, err := models.MigrateRepository(ctxUser, form.RepoName, form.Description, form.Private,
		form.Mirror, u.String())
	if err == nil {
		log.Trace("Repository migrated: %s/%s", ctxUser.Name, form.RepoName)
		ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + form.RepoName)
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

func getForkRepository(ctx *middleware.Context) (*models.Repository, error) {
	forkId := ctx.QueryInt64("fork_id")
	ctx.Data["ForkId"] = forkId

	forkRepo, err := models.GetRepositoryById(forkId)
	if err != nil {
		return nil, fmt.Errorf("GetRepositoryById: %v", err)
	}
	ctx.Data["repo_name"] = forkRepo.Name
	ctx.Data["desc"] = forkRepo.Description

	if err = forkRepo.GetOwner(); err != nil {
		return nil, fmt.Errorf("GetOwner: %v", err)
	}
	ctx.Data["ForkFrom"] = forkRepo.Owner.Name + "/" + forkRepo.Name
	return forkRepo, nil
}

func Fork(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_fork")

	if _, err := getForkRepository(ctx); err != nil {
		if err == models.ErrRepoNotExist {
			ctx.Redirect(setting.AppSubUrl + "/")
		} else {
			ctx.Handle(500, "getForkRepository", err)
		}
		return
	}

	// FIXME: maybe sometime can directly fork to organization?
	ctx.Data["ContextUser"] = ctx.User
	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	ctx.HTML(200, FORK)
}

func ForkPost(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = ctx.Tr("new_fork")

	forkRepo, err := getForkRepository(ctx)
	if err != nil {
		if err == models.ErrRepoNotExist {
			ctx.Redirect(setting.AppSubUrl + "/")
		} else {
			ctx.Handle(500, "getForkRepository", err)
		}
		return
	}

	ctxUser := ctx.User
	// Not equal means current user is an organization.
	if form.Uid != ctx.User.Id {
		var err error
		ctxUser, err = checkContextUser(ctx, form.Uid)
		if err != nil {
			ctx.Handle(500, "checkContextUser", err)
			return
		}
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
		if !ctxUser.IsOwnedBy(ctx.User.Id) {
			ctx.Error(403)
			return
		}
	}

	repo, err := models.ForkRepository(ctxUser, forkRepo, form.RepoName, form.Description)
	if err == nil {
		log.Trace("Repository forked: %s/%s", ctxUser.Name, repo.Name)
		ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + repo.Name)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), FORK, &form)
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
	ctx.Handle(500, "ForkPost", err)
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
		log.Error(4, "Action(%s): %v", ctx.Params(":action"), err)
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
	var (
		uri         = ctx.Params("*")
		refName     string
		ext         string
		archivePath string
		archiveType git.ArchiveType
	)

	switch {
	case strings.HasSuffix(uri, ".zip"):
		ext = ".zip"
		archivePath = path.Join(ctx.Repo.GitRepo.Path, "archives/zip")
		archiveType = git.ZIP
	case strings.HasSuffix(uri, ".tar.gz"):
		ext = ".tar.gz"
		archivePath = path.Join(ctx.Repo.GitRepo.Path, "archives/targz")
		archiveType = git.TARGZ
	default:
		ctx.Error(404)
		return
	}
	refName = strings.TrimSuffix(uri, ext)

	if !com.IsDir(archivePath) {
		if err := os.MkdirAll(archivePath, os.ModePerm); err != nil {
			ctx.Handle(500, "Download -> os.MkdirAll(archivePath)", err)
			return
		}
	}

	// Get corresponding commit.
	var (
		commit *git.Commit
		err    error
	)
	gitRepo := ctx.Repo.GitRepo
	if gitRepo.IsBranchExist(refName) {
		commit, err = gitRepo.GetCommitOfBranch(refName)
		if err != nil {
			ctx.Handle(500, "Download", err)
			return
		}
	} else if gitRepo.IsTagExist(refName) {
		commit, err = gitRepo.GetCommitOfTag(refName)
		if err != nil {
			ctx.Handle(500, "Download", err)
			return
		}
	} else if len(refName) == 40 {
		commit, err = gitRepo.GetCommit(refName)
		if err != nil {
			ctx.Handle(404, "Download", nil)
			return
		}
	} else {
		ctx.Error(404)
		return
	}

	archivePath = path.Join(archivePath, base.ShortSha(commit.Id.String())+ext)
	if !com.IsFile(archivePath) {
		if err := commit.CreateArchive(archivePath, archiveType); err != nil {
			ctx.Handle(500, "Download -> CreateArchive "+archivePath, err)
			return
		}
	}

	ctx.ServeFile(archivePath, ctx.Repo.Repository.Name+"-"+base.ShortSha(commit.Id.String())+ext)
}
