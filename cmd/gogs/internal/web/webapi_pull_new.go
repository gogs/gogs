package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
	"unknwon.dev/x/logx"
)

var (
	pullRequestTemplateFiles      = []string{"PULL_REQUEST.md", ".gogs/PULL_REQUEST.md", ".github/PULL_REQUEST.md"}
	pullRequestTitleTemplateFiles = []string{"PULL_REQUEST_TITLE.md", ".gogs/PULL_REQUEST_TITLE.md", ".github/PULL_REQUEST_TITLE.md"}
)

// pullCompare is what a pull request from the head branch into the base
// branch of the repository would merge.
type pullCompare struct {
	BaseBranch  string
	HeadBranch  string
	HeadOwner   *database.User
	HeadRepo    *database.Repository
	HeadGitRepo *git.Repository
	// Meta is nil when the branches have no common history.
	Meta         *git.PullRequestMeta
	HeadCommitID string
}

// head names the head branch the way the URL of the comparison does: with
// its owner when it is in a fork.
func (p *pullCompare) head(repo *database.Repository) string {
	if p.HeadRepo.ID == repo.ID {
		return p.HeadBranch
	}
	return p.HeadOwner.Name + ":" + p.HeadBranch
}

// comparePull compares the head branch, written as "[<owner>:]<branch>", with
// the base branch of the repository. The head branch is of the repository or
// of the fork its owner has, and only those who can write to it, or site
// admins, can compare it.
func comparePull(ctx context.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, base, head string) (_ *pullCompare, statusCode int, err error) {
	repo := repoCtx.Repo
	errNotExist := errors.New("branch does not exist")
	baseGitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "open repository")
	}
	if !baseGitRepo.HasBranch(ctx, base) {
		return nil, http.StatusNotFound, errNotExist
	}

	p := &pullCompare{BaseBranch: base, HeadBranch: head, HeadOwner: repoCtx.Owner, HeadRepo: repo, HeadGitRepo: baseGitRepo}
	if owner, branch, ok := strings.Cut(head, ":"); ok {
		p.HeadBranch = branch
		if owner != repoCtx.Owner.Name {
			if p.HeadOwner, err = database.Handle.Users().GetByUsername(ctx, owner); err != nil {
				if database.IsErrUserNotExist(err) {
					return nil, http.StatusNotFound, errNotExist
				}
				return nil, http.StatusInternalServerError, errors.Wrap(err, "get head owner")
			}
			var forked bool
			if p.HeadRepo, forked, err = database.HasForkedRepo(p.HeadOwner.ID, repo.ID); err != nil {
				return nil, http.StatusInternalServerError, errors.Wrap(err, "get fork")
			} else if !forked {
				return nil, http.StatusNotFound, errNotExist
			}
			if p.HeadGitRepo, err = git.Open(p.HeadRepo.RepoPath()); err != nil {
				return nil, http.StatusInternalServerError, errors.Wrap(err, "open head repository")
			}
		}
	}
	if !viewer.IsAdmin && !database.Handle.Permissions().Authorize(ctx, logger, viewer.ID, p.HeadRepo.ID, database.AccessModeWrite,
		database.AccessModeOptions{OwnerID: p.HeadRepo.OwnerID, Private: p.HeadRepo.IsPrivate}) {
		return nil, http.StatusNotFound, errNotExist
	}
	if strings.Contains(p.HeadBranch, ":") || !p.HeadGitRepo.HasBranch(ctx, p.HeadBranch) {
		return nil, http.StatusNotFound, errNotExist
	}

	p.Meta, err = p.HeadGitRepo.PullRequestMeta(ctx, database.RepoPath(repoCtx.Owner.Name, repo.Name), p.HeadBranch, base)
	if err != nil {
		if git.IsErrNoMergeBase(err) {
			return p, 0, nil
		}
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get pull request meta")
	}
	if p.HeadCommitID, err = p.HeadGitRepo.BranchCommitID(ctx, p.HeadBranch); err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get head commit")
	}
	return p, 0, nil
}

// openPullIndex returns the index of the open pull request of the same
// branches, or 0 when there is none.
func (p *pullCompare) openPullIndex(repo *database.Repository) (int64, error) {
	pull, err := database.GetUnmergedPullRequest(p.HeadRepo.ID, repo.ID, p.HeadBranch, p.BaseBranch)
	if err != nil {
		if database.IsErrPullRequestNotExist(err) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "get open pull request")
	}
	return pull.Index, nil
}

// readTemplate returns the first of the files that exists in the commit, or
// an empty string.
func readTemplate(ctx context.Context, commit *git.Commit, files []string) (string, error) {
	for _, file := range files {
		blob, err := commit.Blob(ctx, file)
		if err != nil {
			if git.IsErrRevisionNotExist(err) || errors.Is(err, git.ErrNotBlob) {
				continue
			}
			return "", errors.Wrapf(err, "get %q", file)
		}
		p, err := blob.Bytes(ctx)
		if err != nil {
			return "", errors.Wrapf(err, "read %q", file)
		}
		return string(p), nil
	}
	return "", nil
}

type repoNewPull struct {
	BaseBranch string `json:"baseBranch"`
	// Head names the head branch as the URL of the comparison does, with its
	// owner when it is in a fork.
	Head       string `json:"head"`
	HeadOwner  string `json:"headOwner"`
	HeadName   string `json:"headName"`
	HeadBranch string `json:"headBranch"`
	// Branches are those of the repository to merge into, and HeadBranches
	// those of the repository of the head branch.
	Branches     []string `json:"branches"`
	HeadBranches []string `json:"headBranches"`
	// State is "unrelated" when the branches have no common history, "even"
	// when the head branch has nothing to merge, "exists" when an open pull
	// request has the same branches, or "ready".
	State string `json:"state"`
	// PullIndex is the open pull request of the same branches.
	PullIndex int64                `json:"pullIndex,omitempty"`
	Commits   []repoCommitListItem `json:"commits"`
	// Diff is what the pull request would change, unless there is nothing to
	// merge. Its raw text is served by getRepoCompareRaw of its repository.
	Diff *pullDiff `json:"diff,omitempty"`
	// Title and Template prefill the form from the templates of the default
	// branch.
	Title    string `json:"title"`
	Template string `json:"template"`
	// The options are only sent to writers.
	issueMetaOptions
	Attachment     *issueAttachmentSettings `json:"attachment,omitempty"`
	ViewerCanWrite bool                     `json:"viewerCanWrite"`
}

// getRepoNewPull compares the branches in the route, written as
// "<base>...[<owner>:]<head>", to open a pull request from them.
func getRepoNewPull(c flamego.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext) (statusCode int, resp *repoNewPull, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	base, head, ok := strings.Cut(c.Param("range"), "...")
	if !ok {
		return http.StatusNotFound, nil, errors.New("branch does not exist")
	}
	p, statusCode, err := comparePull(ctx, logger, viewer, repoCtx, base, head)
	if err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to compare branches", "range", c.Param("range"), "repoID", repo.ID, "error", err)
		}
		return statusCode, nil, err
	}

	resp = &repoNewPull{
		BaseBranch:     p.BaseBranch,
		Head:           p.head(repo),
		HeadOwner:      p.HeadOwner.Name,
		HeadName:       p.HeadRepo.Name,
		HeadBranch:     p.HeadBranch,
		Commits:        []repoCommitListItem{},
		ViewerCanWrite: repoCtx.Viewer.CanWrite(),
	}
	baseGitRepo, err := openRepoGit(repoCtx)
	if err == nil {
		resp.Branches, err = baseGitRepo.Branches(ctx)
	}
	if err == nil {
		resp.HeadBranches, err = p.HeadGitRepo.Branches(ctx)
	}
	if err != nil {
		logger.Error("Failed to list branches to compare", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}

	switch {
	case p.Meta == nil:
		resp.State = "unrelated"
		return http.StatusOK, resp, nil
	case p.HeadCommitID == p.Meta.MergeBase:
		resp.State = "even"
		return http.StatusOK, resp, nil
	}
	resp.Commits = toCommitListItems(ctx, logger, repo, p.Meta.Commits)
	resp.Diff = &pullDiff{
		Base:               p.Meta.MergeBase,
		Head:               p.HeadCommitID,
		Owner:              p.HeadOwner.Name,
		Name:               p.HeadRepo.Name,
		MarkdownExtensions: conf.Markdown.FileExtensions,
	}
	if resp.PullIndex, err = p.openPullIndex(repo); err != nil {
		logger.Error("Failed to find open pull request", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if resp.PullIndex > 0 {
		resp.State = "exists"
		return http.StatusOK, resp, nil
	}

	resp.State = "ready"
	resp.Attachment = attachmentSettings(issueAttachmentLimits())
	if resp.ViewerCanWrite {
		if resp.issueMetaOptions, err = listIssueMetaOptions(ctx, repo); err != nil {
			logger.Error("Failed to list pull request metadata options", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, err
		}
	}
	// The templates are read from the default branch of the repository to
	// merge into.
	defaultCommit, err := baseGitRepo.BranchCommit(ctx, repo.DefaultBranch)
	if err == nil {
		resp.Template, err = readTemplate(ctx, defaultCommit, pullRequestTemplateFiles)
	}
	if err == nil {
		resp.Title, err = readTemplate(ctx, defaultCommit, pullRequestTitleTemplateFiles)
	}
	if err != nil && !git.IsErrRevisionNotExist(err) {
		logger.Error("Failed to read pull request templates", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "read templates")
	}
	resp.Title = strings.NewReplacer("{{headBranch}}", p.HeadBranch, "{{baseBranch}}", p.BaseBranch).Replace(resp.Title)
	return http.StatusOK, resp, nil
}

type repoNewPullRequest struct {
	repoNewIssueRequest
	// Base and Head are the branches as getRepoNewPull names them.
	Base string `json:"base"`
	Head string `json:"head"`
}

// postRepoPull opens a pull request to merge the head branch into the base
// branch.
func postRepoPull(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, req repoNewPullRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	p, statusCode, err := comparePull(ctx, logger, viewer, repoCtx, req.Base, req.Head)
	if err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to compare branches", "base", req.Base, "head", req.Head, "repoID", repo.ID, "error", err)
		}
		return statusCode, nil, err
	}
	switch {
	case p.Meta == nil:
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("repo.pulls.nothing_merge_base")}, nil
	case p.HeadCommitID == p.Meta.MergeBase:
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("repo.pulls.nothing_to_compare")}, nil
	}
	if index, err := p.openPullIndex(repo); err != nil {
		logger.Error("Failed to find open pull request", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if index > 0 {
		return http.StatusConflict, nil, errors.Newf("pull request #%d has the same branches", index)
	}

	issue, labelIDs, statusCode, resp, err := newIssueFromRequest(c.Request().Context(), l, viewer, repoCtx, req.repoNewIssueRequest)
	if resp != nil || err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check new pull request", "repoID", repo.ID, "error", err)
		}
		return statusCode, resp, err
	}
	issue.IsPull = true

	patch, err := p.HeadGitRepo.DiffBinary(ctx, p.Meta.MergeBase, p.HeadBranch)
	if err != nil {
		logger.Error("Failed to get patch", "branch", p.HeadBranch, "repoID", p.HeadRepo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get patch")
	}
	pull := &database.PullRequest{
		HeadRepoID:   p.HeadRepo.ID,
		BaseRepoID:   repo.ID,
		HeadUserName: p.HeadOwner.Name,
		HeadBranch:   p.HeadBranch,
		BaseBranch:   p.BaseBranch,
		HeadRepo:     p.HeadRepo,
		BaseRepo:     repo,
		MergeBase:    p.Meta.MergeBase,
		Type:         database.PullRequestTypeGogs,
	}
	// Opening the pull request notifies watchers and webhooks about the
	// repository and its owner.
	repo.Owner = repoCtx.Owner
	if err = database.NewPullRequest(ctx, logger, repo, issue, labelIDs, req.Attachments, pull, patch); err != nil {
		logger.Error("Failed to create pull request", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create pull request")
	}
	if err = pull.PushToBaseRepo(ctx, logger); err != nil {
		logger.Error("Failed to push pull request to base repository", "pullID", pull.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "push to base repository")
	}
	return http.StatusCreated, map[string]int64{"index": issue.Index}, nil
}
