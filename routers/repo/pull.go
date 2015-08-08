// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	FORK         base.TplName = "repo/pulls/fork"
	COMPARE_PULL base.TplName = "repo/pulls/compare"
	PULLS        base.TplName = "repo/pulls"
)

func getForkRepository(ctx *middleware.Context) *models.Repository {
	forkRepo, err := models.GetRepositoryByID(ctx.ParamsInt64(":repoid"))
	if err != nil {
		if models.IsErrRepoNotExist(err) {
			ctx.Handle(404, "GetRepositoryByID", nil)
		} else {
			ctx.Handle(500, "GetRepositoryByID", err)
		}
		return nil
	}
	ctx.Data["repo_name"] = forkRepo.Name
	ctx.Data["desc"] = forkRepo.Description
	ctx.Data["IsPrivate"] = forkRepo.IsPrivate

	if err = forkRepo.GetOwner(); err != nil {
		ctx.Handle(500, "GetOwner", err)
		return nil
	}
	ctx.Data["ForkFrom"] = forkRepo.Owner.Name + "/" + forkRepo.Name

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return nil
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	return forkRepo
}

func Fork(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_fork")

	getForkRepository(ctx)
	if ctx.Written() {
		return
	}

	ctx.Data["ContextUser"] = ctx.User
	ctx.HTML(200, FORK)
}

func ForkPost(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = ctx.Tr("new_fork")

	forkRepo := getForkRepository(ctx)
	if ctx.Written() {
		return
	}

	ctxUser := checkContextUser(ctx, form.Uid)
	if ctx.Written() {
		return
	}
	ctx.Data["ContextUser"] = ctxUser

	if ctx.HasError() {
		ctx.HTML(200, FORK)
		return
	}

	repo, has := models.HasForkedRepo(ctxUser.Id, forkRepo.ID)
	if has {
		ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + repo.Name)
		return
	}

	// Check ownership of organization.
	if ctxUser.IsOrganization() {
		if !ctxUser.IsOwnedBy(ctx.User.Id) {
			ctx.Error(403)
			return
		}
	}

	repo, err := models.ForkRepository(ctxUser, forkRepo, form.RepoName, form.Description)
	if err != nil {
		switch {
		case models.IsErrRepoAlreadyExist(err):
			ctx.Data["Err_RepoName"] = true
			ctx.RenderWithErr(ctx.Tr("repo.settings.new_owner_has_same_repo"), FORK, &form)
		case models.IsErrNameReserved(err):
			ctx.Data["Err_RepoName"] = true
			ctx.RenderWithErr(ctx.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), FORK, &form)
		case models.IsErrNamePatternNotAllowed(err):
			ctx.Data["Err_RepoName"] = true
			ctx.RenderWithErr(ctx.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), FORK, &form)
		default:
			ctx.Handle(500, "ForkPost", err)
		}
		return
	}

	log.Trace("Repository forked[%d]: %s/%s", forkRepo.ID, ctxUser.Name, repo.Name)
	ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + repo.Name)
}

func CompareAndPullRequest(ctx *middleware.Context) {
	// Get compare information.
	infos := strings.Split(ctx.Params("*"), "...")
	if len(infos) != 2 {
		ctx.Handle(404, "CompareAndPullRequest", nil)
		return
	}

	baseBranch := infos[0]
	ctx.Data["BaseBranch"] = baseBranch

	headInfos := strings.Split(infos[1], ":")
	if len(headInfos) != 2 {
		ctx.Handle(404, "CompareAndPullRequest", nil)
		return
	}
	headUser := headInfos[0]
	headBranch := headInfos[1]
	ctx.Data["HeadBranch"] = headBranch

	// TODO: check if branches are valid.
	fmt.Println(baseBranch, headUser, headBranch)

	// TODO: add organization support
	// Check if current user has fork of repository.
	headRepo, has := models.HasForkedRepo(ctx.User.Id, ctx.Repo.Repository.ID)
	if !has {
		ctx.Handle(404, "HasForkedRepo", nil)
		return
	}

	headGitRepo, err := git.OpenRepository(models.RepoPath(ctx.User.Name, headRepo.Name))
	if err != nil {
		ctx.Handle(500, "OpenRepository", err)
		return
	}
	headBranches, err := headGitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "GetBranches", err)
		return
	}
	ctx.Data["HeadBranches"] = headBranches

	ctx.HTML(200, COMPARE_PULL)
}

func Pulls(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarPulls"] = true
	ctx.HTML(200, PULLS)
}
