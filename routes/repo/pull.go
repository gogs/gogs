// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"container/list"
	"path"
	"strings"

	"github.com/Unknwon/com"
	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/form"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/tool"
)

const (
	FORK         = "repo/pulls/fork"
	COMPARE_PULL = "repo/pulls/compare"
	PULL_COMMITS = "repo/pulls/commits"
	PULL_FILES   = "repo/pulls/files"

	PULL_REQUEST_TEMPLATE_KEY = "PullRequestTemplate"
)

var (
	PullRequestTemplateCandidates = []string{
		"PULL_REQUEST.md",
		".gogs/PULL_REQUEST.md",
		".github/PULL_REQUEST.md",
	}
)

func parseBaseRepository(c *context.Context) *models.Repository {
	baseRepo, err := models.GetRepositoryByID(c.ParamsInt64(":repoid"))
	if err != nil {
		c.NotFoundOrServerError("GetRepositoryByID", errors.IsRepoNotExist, err)
		return nil
	}

	if !baseRepo.CanBeForked() || !baseRepo.HasAccess(c.User.ID) {
		c.NotFound()
		return nil
	}

	c.Data["repo_name"] = baseRepo.Name
	c.Data["description"] = baseRepo.Description
	c.Data["IsPrivate"] = baseRepo.IsPrivate

	if err = baseRepo.GetOwner(); err != nil {
		c.ServerError("GetOwner", err)
		return nil
	}
	c.Data["ForkFrom"] = baseRepo.Owner.Name + "/" + baseRepo.Name

	if err := c.User.GetOrganizations(true); err != nil {
		c.ServerError("GetOrganizations", err)
		return nil
	}
	c.Data["Orgs"] = c.User.Orgs

	return baseRepo
}

func Fork(c *context.Context) {
	c.Data["Title"] = c.Tr("new_fork")

	parseBaseRepository(c)
	if c.Written() {
		return
	}

	c.Data["ContextUser"] = c.User
	c.Success(FORK)
}

func ForkPost(c *context.Context, f form.CreateRepo) {
	c.Data["Title"] = c.Tr("new_fork")

	baseRepo := parseBaseRepository(c)
	if c.Written() {
		return
	}

	ctxUser := checkContextUser(c, f.UserID)
	if c.Written() {
		return
	}
	c.Data["ContextUser"] = ctxUser

	if c.HasError() {
		c.Success(FORK)
		return
	}

	repo, has, err := models.HasForkedRepo(ctxUser.ID, baseRepo.ID)
	if err != nil {
		c.ServerError("HasForkedRepo", err)
		return
	} else if has {
		c.Redirect(repo.Link())
		return
	}

	// Check ownership of organization.
	if ctxUser.IsOrganization() && !ctxUser.IsOwnedBy(c.User.ID) {
		c.Error(403)
		return
	}

	// Cannot fork to same owner
	if ctxUser.ID == baseRepo.OwnerID {
		c.RenderWithErr(c.Tr("repo.settings.cannot_fork_to_same_owner"), FORK, &f)
		return
	}

	repo, err = models.ForkRepository(c.User, ctxUser, baseRepo, f.RepoName, f.Description)
	if err != nil {
		c.Data["Err_RepoName"] = true
		switch {
		case models.IsErrRepoAlreadyExist(err):
			c.RenderWithErr(c.Tr("repo.settings.new_owner_has_same_repo"), FORK, &f)
		case models.IsErrNameReserved(err):
			c.RenderWithErr(c.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), FORK, &f)
		case models.IsErrNamePatternNotAllowed(err):
			c.RenderWithErr(c.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), FORK, &f)
		default:
			c.ServerError("ForkPost", err)
		}
		return
	}

	log.Trace("Repository forked from '%s' -> '%s'", baseRepo.FullName(), repo.FullName())
	c.Redirect(repo.Link())
}

func checkPullInfo(c *context.Context) *models.Issue {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return nil
	}
	c.Data["Title"] = issue.Title
	c.Data["Issue"] = issue

	if !issue.IsPull {
		c.Handle(404, "ViewPullCommits", nil)
		return nil
	}

	if c.IsLogged {
		// Update issue-user.
		if err = issue.ReadBy(c.User.ID); err != nil {
			c.ServerError("ReadBy", err)
			return nil
		}
	}

	return issue
}

func PrepareMergedViewPullInfo(c *context.Context, issue *models.Issue) {
	pull := issue.PullRequest
	c.Data["HasMerged"] = true
	c.Data["HeadTarget"] = issue.PullRequest.HeadUserName + "/" + pull.HeadBranch
	c.Data["BaseTarget"] = c.Repo.Owner.Name + "/" + pull.BaseBranch

	var err error
	c.Data["NumCommits"], err = c.Repo.GitRepo.CommitsCountBetween(pull.MergeBase, pull.MergedCommitID)
	if err != nil {
		c.ServerError("Repo.GitRepo.CommitsCountBetween", err)
		return
	}
	c.Data["NumFiles"], err = c.Repo.GitRepo.FilesCountBetween(pull.MergeBase, pull.MergedCommitID)
	if err != nil {
		c.ServerError("Repo.GitRepo.FilesCountBetween", err)
		return
	}
}

func PrepareViewPullInfo(c *context.Context, issue *models.Issue) *git.PullRequestInfo {
	repo := c.Repo.Repository
	pull := issue.PullRequest

	c.Data["HeadTarget"] = pull.HeadUserName + "/" + pull.HeadBranch
	c.Data["BaseTarget"] = c.Repo.Owner.Name + "/" + pull.BaseBranch

	var (
		headGitRepo *git.Repository
		err         error
	)

	if pull.HeadRepo != nil {
		headGitRepo, err = git.OpenRepository(pull.HeadRepo.RepoPath())
		if err != nil {
			c.ServerError("OpenRepository", err)
			return nil
		}
	}

	if pull.HeadRepo == nil || !headGitRepo.IsBranchExist(pull.HeadBranch) {
		c.Data["IsPullReuqestBroken"] = true
		c.Data["HeadTarget"] = "deleted"
		c.Data["NumCommits"] = 0
		c.Data["NumFiles"] = 0
		return nil
	}

	prInfo, err := headGitRepo.GetPullRequestInfo(models.RepoPath(repo.Owner.Name, repo.Name),
		pull.BaseBranch, pull.HeadBranch)
	if err != nil {
		if strings.Contains(err.Error(), "fatal: Not a valid object name") {
			c.Data["IsPullReuqestBroken"] = true
			c.Data["BaseTarget"] = "deleted"
			c.Data["NumCommits"] = 0
			c.Data["NumFiles"] = 0
			return nil
		}

		c.ServerError("GetPullRequestInfo", err)
		return nil
	}
	c.Data["NumCommits"] = prInfo.Commits.Len()
	c.Data["NumFiles"] = prInfo.NumFiles
	return prInfo
}

func ViewPullCommits(c *context.Context) {
	c.Data["PageIsPullList"] = true
	c.Data["PageIsPullCommits"] = true

	issue := checkPullInfo(c)
	if c.Written() {
		return
	}
	pull := issue.PullRequest

	if pull.HeadRepo != nil {
		c.Data["Username"] = pull.HeadUserName
		c.Data["Reponame"] = pull.HeadRepo.Name
	}

	var commits *list.List
	if pull.HasMerged {
		PrepareMergedViewPullInfo(c, issue)
		if c.Written() {
			return
		}
		startCommit, err := c.Repo.GitRepo.GetCommit(pull.MergeBase)
		if err != nil {
			c.ServerError("Repo.GitRepo.GetCommit", err)
			return
		}
		endCommit, err := c.Repo.GitRepo.GetCommit(pull.MergedCommitID)
		if err != nil {
			c.ServerError("Repo.GitRepo.GetCommit", err)
			return
		}
		commits, err = c.Repo.GitRepo.CommitsBetween(endCommit, startCommit)
		if err != nil {
			c.ServerError("Repo.GitRepo.CommitsBetween", err)
			return
		}

	} else {
		prInfo := PrepareViewPullInfo(c, issue)
		if c.Written() {
			return
		} else if prInfo == nil {
			c.NotFound()
			return
		}
		commits = prInfo.Commits
	}

	commits = models.ValidateCommitsWithEmails(commits)
	c.Data["Commits"] = commits
	c.Data["CommitsCount"] = commits.Len()

	c.Success(PULL_COMMITS)
}

func ViewPullFiles(c *context.Context) {
	c.Data["PageIsPullList"] = true
	c.Data["PageIsPullFiles"] = true

	issue := checkPullInfo(c)
	if c.Written() {
		return
	}
	pull := issue.PullRequest

	var (
		diffRepoPath  string
		startCommitID string
		endCommitID   string
		gitRepo       *git.Repository
	)

	if pull.HasMerged {
		PrepareMergedViewPullInfo(c, issue)
		if c.Written() {
			return
		}

		diffRepoPath = c.Repo.GitRepo.Path
		startCommitID = pull.MergeBase
		endCommitID = pull.MergedCommitID
		gitRepo = c.Repo.GitRepo
	} else {
		prInfo := PrepareViewPullInfo(c, issue)
		if c.Written() {
			return
		} else if prInfo == nil {
			c.Handle(404, "ViewPullFiles", nil)
			return
		}

		headRepoPath := models.RepoPath(pull.HeadUserName, pull.HeadRepo.Name)

		headGitRepo, err := git.OpenRepository(headRepoPath)
		if err != nil {
			c.ServerError("OpenRepository", err)
			return
		}

		headCommitID, err := headGitRepo.GetBranchCommitID(pull.HeadBranch)
		if err != nil {
			c.ServerError("GetBranchCommitID", err)
			return
		}

		diffRepoPath = headRepoPath
		startCommitID = prInfo.MergeBase
		endCommitID = headCommitID
		gitRepo = headGitRepo
	}

	diff, err := models.GetDiffRange(diffRepoPath,
		startCommitID, endCommitID, setting.Git.MaxGitDiffLines,
		setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles)
	if err != nil {
		c.ServerError("GetDiffRange", err)
		return
	}
	c.Data["Diff"] = diff
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0

	commit, err := gitRepo.GetCommit(endCommitID)
	if err != nil {
		c.ServerError("GetCommit", err)
		return
	}

	setEditorconfigIfExists(c)
	if c.Written() {
		return
	}

	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["IsImageFile"] = commit.IsImageFile

	// It is possible head repo has been deleted for merged pull requests
	if pull.HeadRepo != nil {
		c.Data["Username"] = pull.HeadUserName
		c.Data["Reponame"] = pull.HeadRepo.Name

		headTarget := path.Join(pull.HeadUserName, pull.HeadRepo.Name)
		c.Data["SourcePath"] = setting.AppSubURL + "/" + path.Join(headTarget, "src", endCommitID)
		c.Data["BeforeSourcePath"] = setting.AppSubURL + "/" + path.Join(headTarget, "src", startCommitID)
		c.Data["RawPath"] = setting.AppSubURL + "/" + path.Join(headTarget, "raw", endCommitID)
	}

	c.Data["RequireHighlightJS"] = true
	c.Success(PULL_FILES)
}

func MergePullRequest(c *context.Context) {
	issue := checkPullInfo(c)
	if c.Written() {
		return
	}
	if issue.IsClosed {
		c.NotFound()
		return
	}

	pr, err := models.GetPullRequestByIssueID(issue.ID)
	if err != nil {
		c.NotFoundOrServerError("GetPullRequestByIssueID", models.IsErrPullRequestNotExist, err)
		return
	}

	if !pr.CanAutoMerge() || pr.HasMerged {
		c.NotFound()
		return
	}

	pr.Issue = issue
	pr.Issue.Repo = c.Repo.Repository
	if err = pr.Merge(c.User, c.Repo.GitRepo, models.MergeStyle(c.Query("merge_style"))); err != nil {
		c.ServerError("Merge", err)
		return
	}

	log.Trace("Pull request merged: %d", pr.ID)
	c.Redirect(c.Repo.RepoLink + "/pulls/" + com.ToStr(pr.Index))
}

func ParseCompareInfo(c *context.Context) (*models.User, *models.Repository, *git.Repository, *git.PullRequestInfo, string, string) {
	baseRepo := c.Repo.Repository

	// Get compared branches information
	// format: <base branch>...[<head repo>:]<head branch>
	// base<-head: master...head:feature
	// same repo: master...feature
	infos := strings.Split(c.Params("*"), "...")
	if len(infos) != 2 {
		log.Trace("ParseCompareInfo[%d]: not enough compared branches information %s", baseRepo.ID, infos)
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	baseBranch := infos[0]
	c.Data["BaseBranch"] = baseBranch

	var (
		headUser   *models.User
		headBranch string
		isSameRepo bool
		err        error
	)

	// If there is no head repository, it means pull request between same repository.
	headInfos := strings.Split(infos[1], ":")
	if len(headInfos) == 1 {
		isSameRepo = true
		headUser = c.Repo.Owner
		headBranch = headInfos[0]

	} else if len(headInfos) == 2 {
		headUser, err = models.GetUserByName(headInfos[0])
		if err != nil {
			c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
			return nil, nil, nil, nil, "", ""
		}
		headBranch = headInfos[1]
		isSameRepo = headUser.ID == baseRepo.OwnerID

	} else {
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}
	c.Data["HeadUser"] = headUser
	c.Data["HeadBranch"] = headBranch
	c.Repo.PullRequest.SameRepo = isSameRepo

	// Check if base branch is valid.
	if !c.Repo.GitRepo.IsBranchExist(baseBranch) {
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	var (
		headRepo    *models.Repository
		headGitRepo *git.Repository
	)

	// In case user included redundant head user name for comparison in same repository,
	// no need to check the fork relation.
	if !isSameRepo {
		var has bool
		headRepo, has, err = models.HasForkedRepo(headUser.ID, baseRepo.ID)
		if err != nil {
			c.ServerError("HasForkedRepo", err)
			return nil, nil, nil, nil, "", ""
		} else if !has {
			log.Trace("ParseCompareInfo [base_repo_id: %d]: does not have fork or in same repository", baseRepo.ID)
			c.NotFound()
			return nil, nil, nil, nil, "", ""
		}

		headGitRepo, err = git.OpenRepository(models.RepoPath(headUser.Name, headRepo.Name))
		if err != nil {
			c.ServerError("OpenRepository", err)
			return nil, nil, nil, nil, "", ""
		}
	} else {
		headRepo = c.Repo.Repository
		headGitRepo = c.Repo.GitRepo
	}

	if !c.User.IsWriterOfRepo(headRepo) && !c.User.IsAdmin {
		log.Trace("ParseCompareInfo [base_repo_id: %d]: does not have write access or site admin", baseRepo.ID)
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	// Check if head branch is valid.
	if !headGitRepo.IsBranchExist(headBranch) {
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	headBranches, err := headGitRepo.GetBranches()
	if err != nil {
		c.ServerError("GetBranches", err)
		return nil, nil, nil, nil, "", ""
	}
	c.Data["HeadBranches"] = headBranches

	prInfo, err := headGitRepo.GetPullRequestInfo(models.RepoPath(baseRepo.Owner.Name, baseRepo.Name), baseBranch, headBranch)
	if err != nil {
		if git.IsErrNoMergeBase(err) {
			c.Data["IsNoMergeBase"] = true
			c.Success(COMPARE_PULL)
		} else {
			c.ServerError("GetPullRequestInfo", err)
		}
		return nil, nil, nil, nil, "", ""
	}
	c.Data["BeforeCommitID"] = prInfo.MergeBase

	return headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch
}

func PrepareCompareDiff(
	c *context.Context,
	headUser *models.User,
	headRepo *models.Repository,
	headGitRepo *git.Repository,
	prInfo *git.PullRequestInfo,
	baseBranch, headBranch string) bool {

	var (
		repo = c.Repo.Repository
		err  error
	)

	// Get diff information.
	c.Data["CommitRepoLink"] = headRepo.Link()

	headCommitID, err := headGitRepo.GetBranchCommitID(headBranch)
	if err != nil {
		c.ServerError("GetBranchCommitID", err)
		return false
	}
	c.Data["AfterCommitID"] = headCommitID

	if headCommitID == prInfo.MergeBase {
		c.Data["IsNothingToCompare"] = true
		return true
	}

	diff, err := models.GetDiffRange(models.RepoPath(headUser.Name, headRepo.Name),
		prInfo.MergeBase, headCommitID, setting.Git.MaxGitDiffLines,
		setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles)
	if err != nil {
		c.ServerError("GetDiffRange", err)
		return false
	}
	c.Data["Diff"] = diff
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0

	headCommit, err := headGitRepo.GetCommit(headCommitID)
	if err != nil {
		c.ServerError("GetCommit", err)
		return false
	}

	prInfo.Commits = models.ValidateCommitsWithEmails(prInfo.Commits)
	c.Data["Commits"] = prInfo.Commits
	c.Data["CommitCount"] = prInfo.Commits.Len()
	c.Data["Username"] = headUser.Name
	c.Data["Reponame"] = headRepo.Name
	c.Data["IsImageFile"] = headCommit.IsImageFile

	headTarget := path.Join(headUser.Name, repo.Name)
	c.Data["SourcePath"] = setting.AppSubURL + "/" + path.Join(headTarget, "src", headCommitID)
	c.Data["BeforeSourcePath"] = setting.AppSubURL + "/" + path.Join(headTarget, "src", prInfo.MergeBase)
	c.Data["RawPath"] = setting.AppSubURL + "/" + path.Join(headTarget, "raw", headCommitID)
	return false
}

func CompareAndPullRequest(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.pulls.compare_changes")
	c.Data["PageIsComparePull"] = true
	c.Data["IsDiffCompare"] = true
	c.Data["RequireHighlightJS"] = true
	setTemplateIfExists(c, PULL_REQUEST_TEMPLATE_KEY, PullRequestTemplateCandidates)
	renderAttachmentSettings(c)

	headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch := ParseCompareInfo(c)
	if c.Written() {
		return
	}

	pr, err := models.GetUnmergedPullRequest(headRepo.ID, c.Repo.Repository.ID, headBranch, baseBranch)
	if err != nil {
		if !models.IsErrPullRequestNotExist(err) {
			c.ServerError("GetUnmergedPullRequest", err)
			return
		}
	} else {
		c.Data["HasPullRequest"] = true
		c.Data["PullRequest"] = pr
		c.Success(COMPARE_PULL)
		return
	}

	nothingToCompare := PrepareCompareDiff(c, headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch)
	if c.Written() {
		return
	}

	if !nothingToCompare {
		// Setup information for new form.
		RetrieveRepoMetas(c, c.Repo.Repository)
		if c.Written() {
			return
		}
	}

	setEditorconfigIfExists(c)
	if c.Written() {
		return
	}

	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Success(COMPARE_PULL)
}

func CompareAndPullRequestPost(c *context.Context, f form.NewIssue) {
	c.Data["Title"] = c.Tr("repo.pulls.compare_changes")
	c.Data["PageIsComparePull"] = true
	c.Data["IsDiffCompare"] = true
	c.Data["RequireHighlightJS"] = true
	renderAttachmentSettings(c)

	var (
		repo        = c.Repo.Repository
		attachments []string
	)

	headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch := ParseCompareInfo(c)
	if c.Written() {
		return
	}

	labelIDs, milestoneID, assigneeID := ValidateRepoMetas(c, f)
	if c.Written() {
		return
	}

	if setting.AttachmentEnabled {
		attachments = f.Files
	}

	if c.HasError() {
		form.Assign(f, c.Data)

		// This stage is already stop creating new pull request, so it does not matter if it has
		// something to compare or not.
		PrepareCompareDiff(c, headUser, headRepo, headGitRepo, prInfo, baseBranch, headBranch)
		if c.Written() {
			return
		}

		c.Success(COMPARE_PULL)
		return
	}

	patch, err := headGitRepo.GetPatch(prInfo.MergeBase, headBranch)
	if err != nil {
		c.ServerError("GetPatch", err)
		return
	}

	pullIssue := &models.Issue{
		RepoID:      repo.ID,
		Index:       repo.NextIssueIndex(),
		Title:       f.Title,
		PosterID:    c.User.ID,
		Poster:      c.User,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		IsPull:      true,
		Content:     f.Content,
	}
	pullRequest := &models.PullRequest{
		HeadRepoID:   headRepo.ID,
		BaseRepoID:   repo.ID,
		HeadUserName: headUser.Name,
		HeadBranch:   headBranch,
		BaseBranch:   baseBranch,
		HeadRepo:     headRepo,
		BaseRepo:     repo,
		MergeBase:    prInfo.MergeBase,
		Type:         models.PULL_REQUEST_GOGS,
	}
	// FIXME: check error in the case two people send pull request at almost same time, give nice error prompt
	// instead of 500.
	if err := models.NewPullRequest(repo, pullIssue, labelIDs, attachments, pullRequest, patch); err != nil {
		c.ServerError("NewPullRequest", err)
		return
	} else if err := pullRequest.PushToBaseRepo(); err != nil {
		c.ServerError("PushToBaseRepo", err)
		return
	}

	log.Trace("Pull request created: %d/%d", repo.ID, pullIssue.ID)
	c.Redirect(c.Repo.RepoLink + "/pulls/" + com.ToStr(pullIssue.Index))
}

func parseOwnerAndRepo(c *context.Context) (*models.User, *models.Repository) {
	owner, err := models.GetUserByName(c.Params(":username"))
	if err != nil {
		c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
		return nil, nil
	}

	repo, err := models.GetRepositoryByName(owner.ID, c.Params(":reponame"))
	if err != nil {
		c.NotFoundOrServerError("GetRepositoryByName", errors.IsRepoNotExist, err)
		return nil, nil
	}

	return owner, repo
}

func TriggerTask(c *context.Context) {
	pusherID := c.QueryInt64("pusher")
	branch := c.Query("branch")
	secret := c.Query("secret")
	if len(branch) == 0 || len(secret) == 0 || pusherID <= 0 {
		c.Error(404)
		log.Trace("TriggerTask: branch or secret is empty, or pusher ID is not valid")
		return
	}
	owner, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}
	if secret != tool.MD5(owner.Salt) {
		c.Error(404)
		log.Trace("TriggerTask [%s/%s]: invalid secret", owner.Name, repo.Name)
		return
	}

	pusher, err := models.GetUserByID(pusherID)
	if err != nil {
		c.NotFoundOrServerError("GetUserByID", errors.IsUserNotExist, err)
		return
	}

	log.Trace("TriggerTask '%s/%s' by '%s'", repo.Name, branch, pusher.Name)

	go models.HookQueue.Add(repo.ID)
	go models.AddTestPullRequestTask(pusher, repo.ID, branch, true)
	c.Status(202)
}
