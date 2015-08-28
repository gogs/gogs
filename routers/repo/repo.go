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
)

func checkContextUser(ctx *middleware.Context, uid int64) *models.User {
	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return nil
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	// Not equal means current user is an organization.
	if uid == ctx.User.Id || uid == 0 {
		return ctx.User
	}

	org, err := models.GetUserByID(uid)
	if models.IsErrUserNotExist(err) {
		return ctx.User
	}

	if err != nil {
		ctx.Handle(500, "checkContextUser", fmt.Errorf("GetUserById(%d): %v", uid, err))
		return nil
	}

	// Check ownership of organization.
	if !org.IsOrganization() || !org.IsOwnedBy(ctx.User.Id) {
		ctx.Error(403)
		return nil
	}
	return org
}

func Create(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_repo")

	// Give default value for template to render.
	ctx.Data["Gitignores"] = models.Gitignores
	ctx.Data["Licenses"] = models.Licenses
	ctx.Data["Readmes"] = models.Readmes
	ctx.Data["readme"] = "Default"
	ctx.Data["private"] = ctx.User.LastRepoVisibility

	ctxUser := checkContextUser(ctx, ctx.QueryInt64("org"))
	if ctx.Written() {
		return
	}
	ctx.Data["ContextUser"] = ctxUser

	ctx.HTML(200, CREATE)
}

func handleCreateError(ctx *middleware.Context, err error, name string, tpl base.TplName, form interface{}) {
	switch {
	case models.IsErrRepoAlreadyExist(err):
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), tpl, form)
	case models.IsErrNameReserved(err):
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), tpl, form)
	case models.IsErrNamePatternNotAllowed(err):
		ctx.Data["Err_RepoName"] = true
		ctx.RenderWithErr(ctx.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), tpl, form)
	default:
		ctx.Handle(500, name, err)
	}
}

func CreatePost(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = ctx.Tr("new_repo")

	ctx.Data["Gitignores"] = models.Gitignores
	ctx.Data["Licenses"] = models.Licenses
	ctx.Data["Readmes"] = models.Readmes

	ctxUser := checkContextUser(ctx, form.Uid)
	if ctx.Written() {
		return
	}
	ctx.Data["ContextUser"] = ctxUser

	if ctx.HasError() {
		ctx.HTML(200, CREATE)
		return
	}

	repo, err := models.CreateRepository(ctxUser, models.CreateRepoOptions{
		Name:        form.RepoName,
		Description: form.Description,
		Gitignores:  form.Gitignores,
		License:     form.License,
		Readme:      form.Readme,
		IsPrivate:   form.Private,
		AutoInit:    form.AutoInit,
	})
	if err == nil {
		// Remember visibility preference.
		ctx.User.LastRepoVisibility = repo.IsPrivate
		models.UpdateUser(ctx.User)

		log.Trace("Repository created: %s/%s", ctxUser.Name, repo.Name)
		ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + repo.Name)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.Id, repo.ID, ctxUser.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	handleCreateError(ctx, err, "CreatePost", CREATE, &form)
}

func Migrate(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_migrate")
	ctx.Data["private"] = ctx.User.LastRepoVisibility

	ctxUser := checkContextUser(ctx, ctx.QueryInt64("org"))
	if ctx.Written() {
		return
	}
	ctx.Data["ContextUser"] = ctxUser

	ctx.HTML(200, MIGRATE)
}

func MigratePost(ctx *middleware.Context, form auth.MigrateRepoForm) {
	ctx.Data["Title"] = ctx.Tr("new_migrate")

	ctxUser := checkContextUser(ctx, form.Uid)
	if ctx.Written() {
		return
	}
	ctx.Data["ContextUser"] = ctxUser

	if ctx.HasError() {
		ctx.HTML(200, MIGRATE)
		return
	}

	// Remote address can be HTTP/HTTPS/Git URL or local path.
	// Note: remember to change api/v1/repo.go: MigrateRepo
	// FIXME: merge these two functions with better error handling
	remoteAddr := form.CloneAddr
	if strings.HasPrefix(form.CloneAddr, "http://") ||
		strings.HasPrefix(form.CloneAddr, "https://") ||
		strings.HasPrefix(form.CloneAddr, "git://") {
		u, err := url.Parse(form.CloneAddr)
		if err != nil {
			ctx.Data["Err_CloneAddr"] = true
			ctx.RenderWithErr(ctx.Tr("form.url_error"), MIGRATE, &form)
			return
		}
		if len(form.AuthUsername) > 0 || len(form.AuthPassword) > 0 {
			u.User = url.UserPassword(form.AuthUsername, form.AuthPassword)
		}
		remoteAddr = u.String()
	} else if !com.IsDir(remoteAddr) {
		ctx.Data["Err_CloneAddr"] = true
		ctx.RenderWithErr(ctx.Tr("repo.migrate.invalid_local_path"), MIGRATE, &form)
		return
	}

	repo, err := models.MigrateRepository(ctxUser, form.RepoName, form.Description, form.Private, form.Mirror, remoteAddr)
	if err == nil {
		// Remember visibility preference.
		ctx.User.LastRepoVisibility = repo.IsPrivate
		models.UpdateUser(ctx.User)

		log.Trace("Repository migrated: %s/%s", ctxUser.Name, form.RepoName)
		ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + form.RepoName)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.Id, repo.ID, ctxUser.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	if strings.Contains(err.Error(), "Authentication failed") ||
		strings.Contains(err.Error(), " not found") {
		ctx.Data["Err_Auth"] = true
		ctx.RenderWithErr(ctx.Tr("form.auth_failed", strings.Replace(err.Error(), ":"+form.AuthPassword+"@", ":<password>@", 1)), MIGRATE, &form)
		return
	}

	handleCreateError(ctx, err, "MigratePost", MIGRATE, &form)
}

func Action(ctx *middleware.Context) {
	var err error
	switch ctx.Params(":action") {
	case "watch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.ID, true)
	case "unwatch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.ID, false)
	case "star":
		err = models.StarRepo(ctx.User.Id, ctx.Repo.Repository.ID, true)
	case "unstar":
		err = models.StarRepo(ctx.User.Id, ctx.Repo.Repository.ID, false)
	case "desc":
		if !ctx.Repo.IsOwner() {
			ctx.Error(404)
			return
		}

		ctx.Repo.Repository.Description = ctx.Query("desc")
		ctx.Repo.Repository.Website = ctx.Query("site")
		err = models.UpdateRepository(ctx.Repo.Repository, false)
	}

	if err != nil {
		log.Error(4, "Action(%s): %v", ctx.Params(":action"), err)
		ctx.JSON(200, map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}

	redirectTo := ctx.Query("redirect_to")
	if len(redirectTo) == 0 {
		redirectTo = ctx.Repo.RepoLink
	}
	ctx.Redirect(redirectTo)

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
