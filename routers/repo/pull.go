// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
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

	// Cannot fork bare repo.
	if forkRepo.IsBare {
		ctx.Handle(404, "", nil)
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
		ctx.Data["Err_RepoName"] = true
		switch {
		case models.IsErrRepoAlreadyExist(err):
			ctx.RenderWithErr(ctx.Tr("repo.settings.new_owner_has_same_repo"), FORK, &form)
		case models.IsErrNameReserved(err):
			ctx.RenderWithErr(ctx.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), FORK, &form)
		case models.IsErrNamePatternNotAllowed(err):
			ctx.RenderWithErr(ctx.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), FORK, &form)
		default:
			ctx.Handle(500, "ForkPost", err)
		}
		return
	}

	log.Trace("Repository forked[%d]: %s/%s", forkRepo.ID, ctxUser.Name, repo.Name)
	ctx.Redirect(setting.AppSubUrl + "/" + ctxUser.Name + "/" + repo.Name)
}

func Pulls(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarPulls"] = true
	ctx.HTML(200, PULLS)
}

// func ViewPull

func ParseCompareInfo(ctx *middleware.Context) (*models.User, *models.Repository, *git.Repository, *git.PullRequestInfo, string, string) {
	// Get compare branch information.
	infos := strings.Split(ctx.Params("*"), "...")
	if len(infos) != 2 {
		ctx.Handle(404, "CompareAndPullRequest", nil)
		return nil, nil, nil, nil, "", ""
	}

	baseBranch := infos[0]
	ctx.Data["BaseBranch"] = baseBranch

	headInfos := strings.Split(infos[1], ":")
	if len(headInfos) != 2 {
		ctx.Handle(404, "CompareAndPullRequest", nil)
		return nil, nil, nil, nil, "", ""
	}
	headUsername := headInfos[0]
	headBranch := headInfos[1]
	ctx.Data["HeadBranch"] = headBranch

	headUser, err := models.GetUserByName(headUsername)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Handle(404, "GetUserByName", nil)
		} else {
			ctx.Handle(500, "GetUserByName", err)
		}
		return nil, nil, nil, nil, "", ""
	}

	repo := ctx.Repo.Repository

	// Check if base branch is valid.
	if !ctx.Repo.GitRepo.IsBranchExist(baseBranch) {
		ctx.Handle(404, "IsBranchExist", nil)
		return nil, nil, nil, nil, "", ""
	}

	// Check if current user has fork of repository.
	headRepo, has := models.HasForkedRepo(headUser.Id, repo.ID)
	if !has || !ctx.User.IsAdminOfRepo(headRepo) {
		ctx.Handle(404, "HasForkedRepo", nil)
		return nil, nil, nil, nil, "", ""
	}

	headGitRepo, err := git.OpenRepository(models.RepoPath(headUser.Name, headRepo.Name))
	if err != nil {
		ctx.Handle(500, "OpenRepository", err)
		return nil, nil, nil, nil, "", ""
	}

	// Check if head branch is valid.
	if !headGitRepo.IsBranchExist(headBranch) {
		ctx.Handle(404, "IsBranchExist", nil)
		return nil, nil, nil, nil, "", ""
	}

	headBranches, err := headGitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "GetBranches", err)
		return nil, nil, nil, nil, "", ""
	}
	ctx.Data["HeadBranches"] = headBranches

	prInfo, err := headGitRepo.GetPullRequestInfo(models.RepoPath(repo.Owner.Name, repo.Name), baseBranch, headBranch)
	if err != nil {
		ctx.Handle(500, "GetPullRequestInfo", err)
		return nil, nil, nil, nil, "", ""
	}
	ctx.Data["BeforeCommitID"] = prInfo.MergeBase

	return headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch
}

func PrepareCompareDiff(
	ctx *middleware.Context,
	headUser *models.User,
	headRepo *models.Repository,
	headGitRepo *git.Repository,
	prInfo *git.PullRequestInfo,
	baseBranch, headBranch string) {

	var (
		repo = ctx.Repo.Repository
		err  error
	)

	// Get diff information.
	ctx.Data["CommitRepoLink"], err = headRepo.RepoLink()
	if err != nil {
		ctx.Handle(500, "RepoLink", err)
		return
	}

	headCommitID, err := headGitRepo.GetCommitIdOfBranch(headBranch)
	if err != nil {
		ctx.Handle(500, "GetCommitIdOfBranch", err)
		return
	}
	ctx.Data["AfterCommitID"] = headCommitID

	diff, err := models.GetDiffRange(models.RepoPath(headUser.Name, headRepo.Name),
		prInfo.MergeBase, headCommitID, setting.Git.MaxGitDiffLines)
	if err != nil {
		ctx.Handle(500, "GetDiffRange", err)
		return
	}
	ctx.Data["Diff"] = diff
	ctx.Data["DiffNotAvailable"] = diff.NumFiles() == 0

	headCommit, err := headGitRepo.GetCommit(headCommitID)
	if err != nil {
		ctx.Handle(500, "GetCommit", err)
		return
	}
	isImageFile := func(name string) bool {
		blob, err := headCommit.GetBlobByPath(name)
		if err != nil {
			return false
		}

		dataRc, err := blob.Data()
		if err != nil {
			return false
		}
		buf := make([]byte, 1024)
		n, _ := dataRc.Read(buf)
		if n > 0 {
			buf = buf[:n]
		}
		_, isImage := base.IsImageFile(buf)
		return isImage
	}

	prInfo.Commits = models.ValidateCommitsWithEmails(prInfo.Commits)
	ctx.Data["Commits"] = prInfo.Commits
	ctx.Data["CommitCount"] = prInfo.Commits.Len()
	ctx.Data["Username"] = headUser.Name
	ctx.Data["Reponame"] = headRepo.Name
	ctx.Data["IsImageFile"] = isImageFile
	ctx.Data["SourcePath"] = setting.AppSubUrl + "/" + path.Join(headUser.Name, repo.Name, "src", headCommitID)
	ctx.Data["BeforeSourcePath"] = setting.AppSubUrl + "/" + path.Join(headUser.Name, repo.Name, "src", prInfo.MergeBase)
	ctx.Data["RawPath"] = setting.AppSubUrl + "/" + path.Join(headUser.Name, repo.Name, "raw", headCommitID)
}

func CompareAndPullRequest(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.pulls.compare_changes")
	ctx.Data["PageIsComparePull"] = true
	ctx.Data["IsDiffCompare"] = true
	renderAttachmentSettings(ctx)

	headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch := ParseCompareInfo(ctx)
	if ctx.Written() {
		return
	}

	PrepareCompareDiff(ctx, headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch)
	if ctx.Written() {
		return
	}

	// Setup information for new form.
	RetrieveRepoMetas(ctx, ctx.Repo.Repository)
	if ctx.Written() {
		return
	}

	ctx.HTML(200, COMPARE_PULL)
}

func CompareAndPullRequestPost(ctx *middleware.Context, form auth.CreateIssueForm) {
	ctx.Data["Title"] = ctx.Tr("repo.pulls.compare_changes")
	ctx.Data["PageIsComparePull"] = true
	ctx.Data["IsDiffCompare"] = true
	renderAttachmentSettings(ctx)

	var (
		repo        = ctx.Repo.Repository
		attachments []string
	)

	headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch := ParseCompareInfo(ctx)
	if ctx.Written() {
		return
	}

	patch, err := headGitRepo.GetPatch(models.RepoPath(repo.Owner.Name, repo.Name), baseBranch, headBranch)
	if err != nil {
		ctx.Handle(500, "GetPatch", err)
		return
	}

	labelIDs, milestoneID, assigneeID := ValidateRepoMetas(ctx, form)
	if ctx.Written() {
		return
	}

	if setting.AttachmentEnabled {
		attachments = form.Attachments
	}

	if ctx.HasError() {
		ctx.HTML(200, COMPARE_PULL)
		return
	}

	pr := &models.Issue{
		RepoID:      repo.ID,
		Index:       int64(repo.NumIssues) + 1,
		Name:        form.Title,
		PosterID:    ctx.User.Id,
		Poster:      ctx.User,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		IsPull:      true,
		Content:     form.Content,
	}
	if err := models.NewPullRequest(repo, pr, labelIDs, attachments, &models.PullRepo{
		HeadRepoID:   headRepo.ID,
		BaseRepoID:   repo.ID,
		HeadUserName: headUser.Name,
		HeadBarcnh:   headBranch,
		BaseBranch:   baseBranch,
		MergeBase:    prInfo.MergeBase,
		Type:         models.PULL_REQUEST_GOGS,
	}, patch); err != nil {
		ctx.Handle(500, "NewPullRequest", err)
		return
	}

	log.Trace("Pull request created: %d/%d", repo.ID, pr.ID)
	ctx.Redirect(ctx.Repo.RepoLink + "/pulls/" + com.ToStr(pr.Index))
}
