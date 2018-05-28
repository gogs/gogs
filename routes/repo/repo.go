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
	log "gopkg.in/clog.v1"

	"github.com/gogs/git-module"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/tool"
)

const (
	CREATE  = "repo/create"
	MIGRATE = "repo/migrate"
)

func MustBeNotBare(c *context.Context) {
	if c.Repo.Repository.IsBare {
		c.Handle(404, "MustBeNotBare", nil)
	}
}

func checkContextUser(c *context.Context, uid int64) *models.User {
	orgs, err := models.GetOwnedOrgsByUserIDDesc(c.User.ID, "updated_unix")
	if err != nil {
		c.Handle(500, "GetOwnedOrgsByUserIDDesc", err)
		return nil
	}
	c.Data["Orgs"] = orgs

	// Not equal means current user is an organization.
	if uid == c.User.ID || uid == 0 {
		return c.User
	}

	org, err := models.GetUserByID(uid)
	if errors.IsUserNotExist(err) {
		return c.User
	}

	if err != nil {
		c.Handle(500, "GetUserByID", fmt.Errorf("[%d]: %v", uid, err))
		return nil
	}

	// Check ownership of organization.
	if !org.IsOrganization() || !(c.User.IsAdmin || org.IsOwnedBy(c.User.ID)) {
		c.Error(403)
		return nil
	}
	return org
}

func Create(c *context.Context) {
	c.Data["Title"] = c.Tr("new_repo")

	// Give default value for template to render.
	c.Data["Gitignores"] = models.Gitignores
	c.Data["Licenses"] = models.Licenses
	c.Data["Readmes"] = models.Readmes
	c.Data["readme"] = "Default"
	c.Data["private"] = c.User.LastRepoVisibility
	c.Data["IsForcedPrivate"] = setting.Repository.ForcePrivate

	ctxUser := checkContextUser(c, c.QueryInt64("org"))
	if c.Written() {
		return
	}
	c.Data["ContextUser"] = ctxUser

	c.HTML(200, CREATE)
}

func handleCreateError(c *context.Context, owner *models.User, err error, name, tpl string, form interface{}) {
	switch {
	case errors.IsReachLimitOfRepo(err):
		c.RenderWithErr(c.Tr("repo.form.reach_limit_of_creation", owner.RepoCreationNum()), tpl, form)
	case models.IsErrRepoAlreadyExist(err):
		c.Data["Err_RepoName"] = true
		c.RenderWithErr(c.Tr("form.repo_name_been_taken"), tpl, form)
	case models.IsErrNameReserved(err):
		c.Data["Err_RepoName"] = true
		c.RenderWithErr(c.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), tpl, form)
	case models.IsErrNamePatternNotAllowed(err):
		c.Data["Err_RepoName"] = true
		c.RenderWithErr(c.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), tpl, form)
	default:
		c.Handle(500, name, err)
	}
}

func CreatePost(c *context.Context, f form.CreateRepo) {
	c.Data["Title"] = c.Tr("new_repo")

	c.Data["Gitignores"] = models.Gitignores
	c.Data["Licenses"] = models.Licenses
	c.Data["Readmes"] = models.Readmes

	ctxUser := checkContextUser(c, f.UserID)
	if c.Written() {
		return
	}
	c.Data["ContextUser"] = ctxUser

	if c.HasError() {
		c.HTML(200, CREATE)
		return
	}

	repo, err := models.CreateRepository(c.User, ctxUser, models.CreateRepoOptions{
		Name:        f.RepoName,
		Description: f.Description,
		Gitignores:  f.Gitignores,
		License:     f.License,
		Readme:      f.Readme,
		IsPrivate:   f.Private || setting.Repository.ForcePrivate,
		AutoInit:    f.AutoInit,
	})
	if err == nil {
		log.Trace("Repository created [%d]: %s/%s", repo.ID, ctxUser.Name, repo.Name)
		c.Redirect(setting.AppSubURL + "/" + ctxUser.Name + "/" + repo.Name)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.ID, repo.ID); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	handleCreateError(c, ctxUser, err, "CreatePost", CREATE, &f)
}

func Migrate(c *context.Context) {
	c.Data["Title"] = c.Tr("new_migrate")
	c.Data["private"] = c.User.LastRepoVisibility
	c.Data["IsForcedPrivate"] = setting.Repository.ForcePrivate
	c.Data["mirror"] = c.Query("mirror") == "1"

	ctxUser := checkContextUser(c, c.QueryInt64("org"))
	if c.Written() {
		return
	}
	c.Data["ContextUser"] = ctxUser

	c.HTML(200, MIGRATE)
}

func MigratePost(c *context.Context, f form.MigrateRepo) {
	c.Data["Title"] = c.Tr("new_migrate")

	ctxUser := checkContextUser(c, f.Uid)
	if c.Written() {
		return
	}
	c.Data["ContextUser"] = ctxUser

	if c.HasError() {
		c.HTML(200, MIGRATE)
		return
	}

	remoteAddr, err := f.ParseRemoteAddr(c.User)
	if err != nil {
		if models.IsErrInvalidCloneAddr(err) {
			c.Data["Err_CloneAddr"] = true
			addrErr := err.(models.ErrInvalidCloneAddr)
			switch {
			case addrErr.IsURLError:
				c.RenderWithErr(c.Tr("form.url_error"), MIGRATE, &f)
			case addrErr.IsPermissionDenied:
				c.RenderWithErr(c.Tr("repo.migrate.permission_denied"), MIGRATE, &f)
			case addrErr.IsInvalidPath:
				c.RenderWithErr(c.Tr("repo.migrate.invalid_local_path"), MIGRATE, &f)
			default:
				c.Handle(500, "Unknown error", err)
			}
		} else {
			c.Handle(500, "ParseRemoteAddr", err)
		}
		return
	}

	repo, err := models.MigrateRepository(c.User, ctxUser, models.MigrateRepoOptions{
		Name:        f.RepoName,
		Description: f.Description,
		IsPrivate:   f.Private || setting.Repository.ForcePrivate,
		IsMirror:    f.Mirror,
		RemoteAddr:  remoteAddr,
	})
	if err == nil {
		log.Trace("Repository migrated [%d]: %s/%s", repo.ID, ctxUser.Name, f.RepoName)
		c.Redirect(setting.AppSubURL + "/" + ctxUser.Name + "/" + f.RepoName)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.ID, repo.ID); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	if strings.Contains(err.Error(), "Authentication failed") ||
		strings.Contains(err.Error(), "could not read Username") {
		c.Data["Err_Auth"] = true
		c.RenderWithErr(c.Tr("form.auth_failed", models.HandleMirrorCredentials(err.Error(), true)), MIGRATE, &f)
		return
	} else if strings.Contains(err.Error(), "fatal:") {
		c.Data["Err_CloneAddr"] = true
		c.RenderWithErr(c.Tr("repo.migrate.failed", models.HandleMirrorCredentials(err.Error(), true)), MIGRATE, &f)
		return
	}

	handleCreateError(c, ctxUser, err, "MigratePost", MIGRATE, &f)
}

func Action(c *context.Context) {
	var err error
	switch c.Params(":action") {
	case "watch":
		err = models.WatchRepo(c.User.ID, c.Repo.Repository.ID, true)
	case "unwatch":
		err = models.WatchRepo(c.User.ID, c.Repo.Repository.ID, false)
	case "star":
		err = models.StarRepo(c.User.ID, c.Repo.Repository.ID, true)
	case "unstar":
		err = models.StarRepo(c.User.ID, c.Repo.Repository.ID, false)
	case "desc": // FIXME: this is not used
		if !c.Repo.IsOwner() {
			c.Error(404)
			return
		}

		c.Repo.Repository.Description = c.Query("desc")
		c.Repo.Repository.Website = c.Query("site")
		err = models.UpdateRepository(c.Repo.Repository, false)
	}

	if err != nil {
		c.Handle(500, fmt.Sprintf("Action (%s)", c.Params(":action")), err)
		return
	}

	redirectTo := c.Query("redirect_to")
	if len(redirectTo) == 0 {
		redirectTo = c.Repo.RepoLink
	}
	c.Redirect(redirectTo)
}

func Download(c *context.Context) {
	var (
		uri         = c.Params("*")
		refName     string
		ext         string
		archivePath string
		archiveType git.ArchiveType
	)

	switch {
	case strings.HasSuffix(uri, ".zip"):
		ext = ".zip"
		archivePath = path.Join(c.Repo.GitRepo.Path, "archives/zip")
		archiveType = git.ZIP
	case strings.HasSuffix(uri, ".tar.gz"):
		ext = ".tar.gz"
		archivePath = path.Join(c.Repo.GitRepo.Path, "archives/targz")
		archiveType = git.TARGZ
	default:
		log.Trace("Unknown format: %s", uri)
		c.Error(404)
		return
	}
	refName = strings.TrimSuffix(uri, ext)

	if !com.IsDir(archivePath) {
		if err := os.MkdirAll(archivePath, os.ModePerm); err != nil {
			c.Handle(500, "Download -> os.MkdirAll(archivePath)", err)
			return
		}
	}

	// Get corresponding commit.
	var (
		commit *git.Commit
		err    error
	)
	gitRepo := c.Repo.GitRepo
	if gitRepo.IsBranchExist(refName) {
		commit, err = gitRepo.GetBranchCommit(refName)
		if err != nil {
			c.Handle(500, "GetBranchCommit", err)
			return
		}
	} else if gitRepo.IsTagExist(refName) {
		commit, err = gitRepo.GetTagCommit(refName)
		if err != nil {
			c.Handle(500, "GetTagCommit", err)
			return
		}
	} else if len(refName) >= 7 && len(refName) <= 40 {
		commit, err = gitRepo.GetCommit(refName)
		if err != nil {
			c.NotFound()
			return
		}
	} else {
		c.NotFound()
		return
	}

	archivePath = path.Join(archivePath, tool.ShortSHA1(commit.ID.String())+ext)
	if !com.IsFile(archivePath) {
		if err := commit.CreateArchive(archivePath, archiveType); err != nil {
			c.Handle(500, "Download -> CreateArchive "+archivePath, err)
			return
		}
	}

	c.ServeFile(archivePath, c.Repo.Repository.Name+"-"+refName+ext)
}
