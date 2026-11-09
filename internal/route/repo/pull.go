package repo

import (
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/git"
)

const (
	tmplRepoPullsCompare = "repo/pulls/compare"
	tmplRepoPullsCommits = "repo/pulls/commits"
	tmplRepoPullsFiles   = "repo/pulls/files"

	PullRequestTemplateKey      = "PullRequestTemplate"
	PullRequestTitleTemplateKey = "PullRequestTitleTemplate"
)

var (
	PullRequestTemplateCandidates = []string{
		"PULL_REQUEST.md",
		".gogs/PULL_REQUEST.md",
		".github/PULL_REQUEST.md",
	}

	PullRequestTitleTemplateCandidates = []string{
		"PULL_REQUEST_TITLE.md",
		".gogs/PULL_REQUEST_TITLE.md",
		".github/PULL_REQUEST_TITLE.md",
	}
)

func checkPullInfo(c *context.Context) *database.Issue {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return nil
	}
	c.Data["Title"] = issue.Title
	c.Data["Issue"] = issue

	if !issue.IsPull {
		c.NotFound()
		return nil
	}

	if c.IsLogged {
		// Update issue-user.
		if err = issue.ReadBy(c.User.ID); err != nil {
			c.Error(err, "mark read by")
			return nil
		}
	}

	return issue
}

func PrepareMergedViewPullInfo(c *context.Context, issue *database.Issue) {
	pull := issue.PullRequest
	c.Data["HasMerged"] = true
	c.Data["HeadTarget"] = issue.PullRequest.HeadUserName + "/" + pull.HeadBranch
	c.Data["BaseTarget"] = c.Repo.Owner.Name + "/" + pull.BaseBranch

	var err error
	c.Data["NumCommits"], err = c.Repo.GitRepo.RevListCount(c.Req.Context(), []string{pull.MergeBase + "..." + pull.MergedCommitID})
	if err != nil {
		c.Error(err, "count commits")
		return
	}

	names, err := c.Repo.GitRepo.DiffNameOnly(c.Req.Context(), pull.MergeBase, pull.MergedCommitID, git.DiffNameOnlyOptions{NeedsMergeBase: true})
	c.Data["NumFiles"] = len(names)
	if err != nil {
		c.Error(err, "get changed files")
		return
	}
}

func PrepareViewPullInfo(c *context.Context, issue *database.Issue) *git.PullRequestMeta {
	repo := c.Repo.Repository
	pull := issue.PullRequest

	c.Data["HeadTarget"] = pull.HeadUserName + "/" + pull.HeadBranch
	c.Data["BaseTarget"] = c.Repo.Owner.Name + "/" + pull.BaseBranch

	var (
		headGitRepo *git.Repository
		err         error
	)

	if pull.HeadRepo != nil {
		headGitRepo, err = git.Open(pull.HeadRepo.RepoPath())
		if err != nil {
			c.Error(err, "open repository")
			return nil
		}
	}

	if pull.HeadRepo == nil || !headGitRepo.HasBranch(c.Req.Context(), pull.HeadBranch) {
		c.Data["IsPullReuqestBroken"] = true
		c.Data["HeadTarget"] = "deleted"
		c.Data["NumCommits"] = 0
		c.Data["NumFiles"] = 0
		return nil
	}

	baseRepoPath := database.RepoPath(repo.Owner.Name, repo.Name)
	if !git.RepoHasBranch(c.Req.Context(), baseRepoPath, pull.BaseBranch) {
		c.Data["IsPullReuqestBroken"] = true
		c.Data["BaseTarget"] = "deleted"
		c.Data["NumCommits"] = 0
		c.Data["NumFiles"] = 0
		return nil
	}

	prMeta, err := headGitRepo.PullRequestMeta(c.Req.Context(), baseRepoPath, pull.HeadBranch, pull.BaseBranch)
	if err != nil {
		c.Error(err, "get pull request meta")
		return nil
	}
	c.Data["NumCommits"] = len(prMeta.Commits)
	c.Data["NumFiles"] = prMeta.NumFiles
	return prMeta
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

	var commits []*git.Commit
	if pull.HasMerged {
		PrepareMergedViewPullInfo(c, issue)
		if c.Written() {
			return
		}
		startCommit, err := c.Repo.GitRepo.CatFileCommit(c.Req.Context(), pull.MergeBase)
		if err != nil {
			c.Error(err, "get commit of merge base")
			return
		}
		endCommit, err := c.Repo.GitRepo.CatFileCommit(c.Req.Context(), pull.MergedCommitID)
		if err != nil {
			c.Error(err, "get merged commit")
			return
		}
		commits, err = c.Repo.GitRepo.RevList(c.Req.Context(), []string{startCommit.ID.String() + "..." + endCommit.ID.String()})
		if err != nil {
			c.Error(err, "list commits")
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

	c.Data["Commits"] = matchUsersWithCommitEmails(c.Req.Context(), commits)
	c.Data["CommitsCount"] = len(commits)

	c.Success(tmplRepoPullsCommits)
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
		diffGitRepo   *git.Repository
		startCommitID string
		endCommitID   string
		gitRepo       *git.Repository
	)

	if pull.HasMerged {
		PrepareMergedViewPullInfo(c, issue)
		if c.Written() {
			return
		}

		diffGitRepo = c.Repo.GitRepo
		startCommitID = pull.MergeBase
		endCommitID = pull.MergedCommitID
		gitRepo = c.Repo.GitRepo
	} else {
		prInfo := PrepareViewPullInfo(c, issue)
		if c.Written() {
			return
		} else if prInfo == nil {
			c.NotFound()
			return
		}

		headRepoPath := database.RepoPath(pull.HeadUserName, pull.HeadRepo.Name)

		headGitRepo, err := git.Open(headRepoPath)
		if err != nil {
			c.Error(err, "open repository")
			return
		}

		headCommitID, err := headGitRepo.BranchCommitID(c.Req.Context(), pull.HeadBranch)
		if err != nil {
			c.Error(err, "get head branch commit ID")
			return
		}

		diffGitRepo = headGitRepo
		startCommitID = prInfo.MergeBase
		endCommitID = headCommitID
		gitRepo = headGitRepo
	}

	ctx, cancel := git.WithTimeout(c.Req.Context(), time.Duration(conf.Git.Timeout.Diff)*time.Second)
	defer cancel()
	diff, err := diffGitRepo.Diff(ctx,
		endCommitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
		git.DiffOptions{Base: startCommitID},
	)
	if err != nil {
		c.Error(err, "get diff")
		return
	}
	c.Data["Diff"] = diff
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0

	commit, err := gitRepo.CatFileCommit(c.Req.Context(), endCommitID)
	if err != nil {
		c.Error(err, "get commit")
		return
	}

	setEditorconfigIfExists(c)
	if c.Written() {
		return
	}

	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["IsImageFile"] = func(subpath string) (bool, error) {
		return commit.IsImageFile(c.Req.Context(), subpath)
	}
	c.Data["IsImageFileByIndex"] = func(index string) (bool, error) {
		return commit.IsImageFileByIndex(c.Req.Context(), index)
	}

	// It is possible head repo has been deleted for merged pull requests
	if pull.HeadRepo != nil {
		c.Data["Username"] = pull.HeadUserName
		c.Data["Reponame"] = pull.HeadRepo.Name

		headTarget := path.Join(pull.HeadUserName, pull.HeadRepo.Name)
		c.Data["SourcePath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "src", endCommitID)
		c.Data["RawPath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "raw", endCommitID)
		c.Data["BeforeSourcePath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "src", startCommitID)
		c.Data["BeforeRawPath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "raw", startCommitID)
	}

	c.Data["RequireHighlightJS"] = true
	c.Success(tmplRepoPullsFiles)
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

	pr, err := database.GetPullRequestByIssueID(issue.ID)
	if err != nil {
		c.NotFoundOrError(err, "get pull request by issue ID")
		return
	}

	if !pr.CanAutoMerge() || pr.HasMerged {
		c.NotFound()
		return
	}

	pr.Issue = issue
	pr.Issue.Repo = c.Repo.Repository
	if err = pr.Merge(c.Req.Context(), c.User, c.Repo.GitRepo, database.MergeStyle(c.Query("merge_style")), c.Query("commit_description")); err != nil {
		c.Error(err, "merge")
		return
	}

	log.Trace("Pull request merged: %d", pr.ID)
	c.Redirect(c.Repo.RepoLink + "/pulls/" + strconv.FormatInt(pr.Index, 10))
}

func ParseCompareInfo(c *context.Context) (*database.User, *database.Repository, *git.Repository, *git.PullRequestMeta, string, string) {
	baseRepo := c.Repo.Repository

	// Get compared refs information
	// format: <base ref>...[<head repo>:]<head ref>
	// base<-head: master...head:feature
	// same repo: master...feature
	infos := strings.Split(c.Params("*"), "...")
	if len(infos) != 2 {
		log.Trace("ParseCompareInfo[%d]: not enough compared refs information %s", baseRepo.ID, infos)
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	baseRef := infos[0]
	c.Data["BaseBranch"] = baseRef

	var (
		headUser   *database.User
		headRef    string
		isSameRepo bool
		err        error
	)

	// If there is no head repository, it means pull request between same repository.
	headInfos := strings.Split(infos[1], ":")
	if len(headInfos) == 1 {
		isSameRepo = true
		headUser = c.Repo.Owner
		headRef = headInfos[0]

	} else if len(headInfos) == 2 {
		headUser, err = database.Handle.Users().GetByUsername(c.Req.Context(), headInfos[0])
		if err != nil {
			c.NotFoundOrError(err, "get user by name")
			return nil, nil, nil, nil, "", ""
		}
		headRef = headInfos[1]
		isSameRepo = headUser.ID == baseRepo.OwnerID

	} else {
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}
	c.Data["HeadUser"] = headUser
	c.Data["HeadBranch"] = headRef
	c.Repo.PullRequest.SameRepo = isSameRepo

	// Check if base ref is valid.
	if _, err := c.Repo.GitRepo.RevParse(c.Req.Context(), baseRef); err != nil {
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	var (
		headRepo    *database.Repository
		headGitRepo *git.Repository
	)

	// In case user included redundant head user name for comparison in same repository,
	// no need to check the fork relation.
	if !isSameRepo {
		var has bool
		headRepo, has, err = database.HasForkedRepo(headUser.ID, baseRepo.ID)
		if err != nil {
			c.Error(err, "get forked repository")
			return nil, nil, nil, nil, "", ""
		} else if !has {
			log.Trace("ParseCompareInfo [base_repo_id: %d]: does not have fork or in same repository", baseRepo.ID)
			c.NotFound()
			return nil, nil, nil, nil, "", ""
		}

		headGitRepo, err = git.Open(database.RepoPath(headUser.Name, headRepo.Name))
		if err != nil {
			c.Error(err, "open repository")
			return nil, nil, nil, nil, "", ""
		}
	} else {
		headRepo = c.Repo.Repository
		headGitRepo = c.Repo.GitRepo
	}

	if !database.Handle.Permissions().Authorize(
		c.Req.Context(),
		c.User.ID,
		headRepo.ID,
		database.AccessModeWrite,
		database.AccessModeOptions{
			OwnerID: headRepo.OwnerID,
			Private: headRepo.IsPrivate,
		},
	) && !c.User.IsAdmin {
		log.Trace("ParseCompareInfo [base_repo_id: %d]: does not have write access or site admin", baseRepo.ID)
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	// Check if head ref is valid.
	if _, err := headGitRepo.RevParse(c.Req.Context(), headRef); err != nil {
		c.NotFound()
		return nil, nil, nil, nil, "", ""
	}

	headBranches, err := headGitRepo.Branches(c.Req.Context())
	if err != nil {
		c.Error(err, "get branches")
		return nil, nil, nil, nil, "", ""
	}
	c.Data["HeadBranches"] = headBranches

	baseRepoPath := database.RepoPath(baseRepo.Owner.Name, baseRepo.Name)
	meta, err := headGitRepo.PullRequestMeta(c.Req.Context(), baseRepoPath, headRef, baseRef)
	if err != nil {
		if git.IsErrNoMergeBase(err) {
			c.Data["IsNoMergeBase"] = true
			c.Success(tmplRepoPullsCompare)
		} else {
			c.Error(err, "get pull request meta")
		}
		return nil, nil, nil, nil, "", ""
	}
	c.Data["BeforeCommitID"] = meta.MergeBase

	return headUser, headRepo, headGitRepo, meta, baseRef, headRef
}

func PrepareCompareDiff(
	c *context.Context,
	headUser *database.User,
	headRepo *database.Repository,
	headGitRepo *git.Repository,
	meta *git.PullRequestMeta,
	headRef string,
) bool {
	var (
		repo = c.Repo.Repository
		err  error
	)

	// Get diff information.
	c.Data["CommitRepoLink"] = headRepo.Link()

	headCommitID, err := headGitRepo.RevParse(c.Req.Context(), headRef)
	if err != nil {
		c.Error(err, "get head commit ID")
		return false
	}
	c.Data["AfterCommitID"] = headCommitID

	if headCommitID == meta.MergeBase {
		c.Data["IsNothingToCompare"] = true
		return true
	}

	ctx, cancel := git.WithTimeout(c.Req.Context(), time.Duration(conf.Git.Timeout.Diff)*time.Second)
	defer cancel()
	diff, err := headGitRepo.Diff(ctx,
		headCommitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
		git.DiffOptions{Base: meta.MergeBase},
	)
	if err != nil {
		c.Error(err, "get repository diff")
		return false
	}
	c.Data["Diff"] = diff
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0

	headCommit, err := headGitRepo.CatFileCommit(c.Req.Context(), headCommitID)
	if err != nil {
		c.Error(err, "get head commit")
		return false
	}

	c.Data["Commits"] = matchUsersWithCommitEmails(c.Req.Context(), meta.Commits)
	c.Data["CommitCount"] = len(meta.Commits)
	c.Data["Username"] = headUser.Name
	c.Data["Reponame"] = headRepo.Name
	c.Data["IsImageFile"] = func(subpath string) (bool, error) {
		return headCommit.IsImageFile(c.Req.Context(), subpath)
	}
	c.Data["IsImageFileByIndex"] = func(index string) (bool, error) {
		return headCommit.IsImageFileByIndex(c.Req.Context(), index)
	}

	headTarget := path.Join(headUser.Name, repo.Name)
	c.Data["SourcePath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "src", headCommitID)
	c.Data["RawPath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "raw", headCommitID)
	c.Data["BeforeSourcePath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "src", meta.MergeBase)
	c.Data["BeforeRawPath"] = conf.Server.Subpath + "/" + path.Join(headTarget, "raw", meta.MergeBase)
	return false
}

func CompareAndPullRequest(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.pulls.compare_changes")
	c.Data["PageIsComparePull"] = true
	c.Data["IsDiffCompare"] = true
	c.Data["RequireHighlightJS"] = true
	setTemplateIfExists(c, PullRequestTemplateKey, PullRequestTemplateCandidates)
	renderAttachmentSettings(c)

	headUser, headRepo, headGitRepo, prInfo, baseRef, headRef := ParseCompareInfo(c)
	if c.Written() {
		return
	}

	pr, err := database.GetUnmergedPullRequest(headRepo.ID, c.Repo.Repository.ID, headRef, baseRef)
	if err != nil {
		if !database.IsErrPullRequestNotExist(err) {
			c.Error(err, "get unmerged pull request")
			return
		}
	} else {
		c.Data["HasPullRequest"] = true
		c.Data["PullRequest"] = pr
		c.Success(tmplRepoPullsCompare)
		return
	}

	nothingToCompare := PrepareCompareDiff(c, headUser, headRepo, headGitRepo, prInfo, headRef)
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
	setTemplateIfExists(c, PullRequestTitleTemplateKey, PullRequestTitleTemplateCandidates)

	if c.Data[PullRequestTitleTemplateKey] != nil {
		customTitle := c.Data[PullRequestTitleTemplateKey].(string)
		r := strings.NewReplacer("{{headBranch}}", headRef, "{{baseBranch}}", baseRef)
		c.Data["title"] = r.Replace(customTitle)
	}

	c.Success(tmplRepoPullsCompare)
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

	headUser, headRepo, headGitRepo, meta, baseRef, headRef := ParseCompareInfo(c)
	if c.Written() {
		return
	}

	labelIDs, milestoneID, assigneeID := ValidateRepoMetas(c, f)
	if c.Written() {
		return
	}

	if conf.Attachment.Enabled {
		attachments = f.Files
	}

	if c.HasError() {
		form.Assign(f, c.Data)

		// This stage is already stop creating new pull request, so it does not matter if it has
		// something to compare or not.
		PrepareCompareDiff(c, headUser, headRepo, headGitRepo, meta, headRef)
		if c.Written() {
			return
		}

		c.HTML(http.StatusBadRequest, tmplRepoPullsCompare)
		return
	}

	patch, err := headGitRepo.DiffBinary(c.Req.Context(), meta.MergeBase, headRef)
	if err != nil {
		c.Error(err, "get patch")
		return
	}

	pullIssue := &database.Issue{
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
	pullRequest := &database.PullRequest{
		HeadRepoID:   headRepo.ID,
		BaseRepoID:   repo.ID,
		HeadUserName: headUser.Name,
		HeadBranch:   headRef,
		BaseBranch:   baseRef,
		HeadRepo:     headRepo,
		BaseRepo:     repo,
		MergeBase:    meta.MergeBase,
		Type:         database.PullRequestTypeGogs,
	}
	// FIXME: check error in the case two people send pull request at almost same time, give nice error prompt
	// instead of 500.
	if err := database.NewPullRequest(c.Req.Context(), repo, pullIssue, labelIDs, attachments, pullRequest, patch); err != nil {
		c.Error(err, "new pull request")
		return
	} else if err := pullRequest.PushToBaseRepo(c.Req.Context()); err != nil {
		c.Error(err, "push to base repository")
		return
	}

	log.Trace("Pull request created: %d/%d", repo.ID, pullIssue.ID)
	c.Redirect(c.Repo.RepoLink + "/pulls/" + strconv.FormatInt(pullIssue.Index, 10))
}
