package web

import (
	"context"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
	"unknwon.dev/x/logx"
)

// withRepoPulls lets through readers of a repository that takes pull
// requests. The pull requests are not found otherwise.
func withRepoPulls(c flamego.Context, repoCtx *repoContext) {
	if !repoCtx.Viewer.CanRead() || !repoCtx.Repo.AllowsPulls() {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// withRepoIssuesOrPulls lets through viewers who can see the issues or the
// pull requests of the repository, which are what comments are on.
func withRepoIssuesOrPulls(c flamego.Context, repoCtx *repoContext) {
	repo := repoCtx.Repo
	issues := repo.EnableIssues && !repo.EnableExternalTracker && (repoCtx.Viewer.CanRead() || repo.CanGuestViewIssues())
	pulls := repoCtx.Viewer.CanRead() && repo.AllowsPulls()
	if !issues && !pulls {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// withRepoPull injects the pull request in the route, as its issue.
func withRepoPull(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) {
	injectRepoIssue(c, logger, repoCtx, true)
}

type repoPullInfo struct {
	// State is "merged", "closed", "broken" when a branch or the fork is
	// gone, "checking" while conflicts are checked, "mergeable" or
	// "conflict".
	State      string `json:"state"`
	HeadBranch string `json:"headBranch"`
	BaseBranch string `json:"baseBranch"`
	// HeadLabel and BaseLabel name the branches after their owners, or are
	// "deleted" for a branch that is gone.
	HeadLabel string `json:"headLabel"`
	BaseLabel string `json:"baseLabel"`
	// HeadRepoLink is the repository of the head branch, unless it is gone.
	HeadRepoLink string     `json:"headRepoLink,omitempty"`
	CommitCount  int        `json:"commitCount"`
	FileCount    int        `json:"fileCount"`
	Merger       *issueUser `json:"merger,omitempty"`
	MergedAt     *time.Time `json:"mergedAt,omitempty"`
	AllowMerge   bool       `json:"allowMerge"`
	AllowRebase  bool       `json:"allowRebase"`
	AllowSquash  bool       `json:"allowSquash"`
	// ViewerCanMerge is whether the viewer can merge it now.
	ViewerCanMerge bool `json:"viewerCanMerge"`
	// ViewerNotInAllowlist is whether the viewer could merge it if they were
	// in the push allowlist of the protected base branch.
	ViewerNotInAllowlist bool `json:"viewerNotInAllowlist"`
	// ViewerID is the viewer who can merge, whose browser remembers the merge
	// style they last used in the repository.
	ViewerID int64 `json:"viewerID,omitempty"`
	// MergeMessages are the messages the commits of the merge styles that
	// create one start with, for viewers who can merge.
	MergeMessages map[database.MergeStyle]database.MergeMessage `json:"mergeMessages,omitempty"`
	// ViewerCanDeleteBranch is whether the viewer can delete the head branch
	// after the merge: a branch of the same repository that is not protected
	// and has no new commits.
	ViewerCanDeleteBranch bool `json:"viewerCanDeleteBranch"`
	// ViewerCanRestoreBranch is whether the viewer can create the deleted head
	// branch again at the commit that was merged.
	ViewerCanRestoreBranch bool `json:"viewerCanRestoreBranch"`
	// Diff is what the pull request changes, unless it is broken. Its raw
	// text is served by getRepoPullRaw.
	Diff *pullDiff `json:"diff,omitempty"`
}

// pullDiff is the commits the changes of a pull request are between, in the
// repository that has both.
type pullDiff struct {
	Base  string `json:"base"`
	Head  string `json:"head"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
	// MarkdownExtensions are the extensions of files that show a rich diff.
	MarkdownExtensions []string `json:"markdownExtensions"`
}

type repoPullView struct {
	*repoIssueView
	Pull *repoPullInfo `json:"pull"`
	// Threads are the threads of code comments the viewer can see, oldest
	// first.
	Threads []issueComment `json:"threads"`
	*pullReviewStatus
}

// pullRange is the commits a pull request merges, from Base to Head, and the
// repository that has them.
type pullRange struct {
	GitRepo *git.Repository
	// Owner and Name are the repository of GitRepo.
	Owner, Name string
	Base, Head  string
}

// describePull describes the state of the pull request and its branches, and
// returns the commits it merges, which are nil when it is broken.
func describePull(ctx context.Context, repoCtx *repoContext, issue *database.Issue) (info *repoPullInfo, rng *pullRange, commits []*git.Commit, err error) {
	repo := repoCtx.Repo
	pull := issue.PullRequest
	if err = database.Handle.PullRequests().LoadAttributes(ctx, pull); err != nil {
		return nil, nil, nil, errors.Wrap(err, "load attributes")
	}
	info = &repoPullInfo{
		HeadBranch:  pull.HeadBranch,
		BaseBranch:  pull.BaseBranch,
		HeadLabel:   pull.HeadUserName + "/" + pull.HeadBranch,
		BaseLabel:   repoCtx.Owner.Name + "/" + pull.BaseBranch,
		AllowMerge:  repo.PullsAllowMerge,
		AllowRebase: repo.PullsAllowRebase,
		AllowSquash: repo.PullsAllowSquash,
	}
	if pull.HeadRepo != nil {
		info.HeadRepoLink = pull.HeadRepo.Link()
	}

	baseGitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "open repository")
	}
	if pull.HasMerged {
		info.State = "merged"
		mergedAt := pull.Merged.UTC()
		info.MergedAt = &mergedAt
		merger := toIssueUser(pull.Merger)
		info.Merger = &merger
		rng = &pullRange{GitRepo: baseGitRepo, Owner: repoCtx.Owner.Name, Name: repo.Name, Base: pull.MergeBase, Head: pull.MergedCommitID}
		if commits, err = baseGitRepo.RevList(ctx, []string{pull.MergeBase + "..." + pull.MergedCommitID}); err != nil {
			return nil, nil, nil, errors.Wrap(err, "list commits")
		}
		names, err := baseGitRepo.DiffNameOnly(ctx, pull.MergeBase, pull.MergedCommitID, git.DiffNameOnlyOptions{NeedsMergeBase: true})
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "list changed files")
		}
		info.CommitCount, info.FileCount = len(commits), len(names)

		if repoCtx.Viewer.CanWrite() && pull.HeadRepoID == pull.BaseRepoID {
			if baseGitRepo.HasBranch(ctx, pull.HeadBranch) {
				protected, err := database.Handle.ProtectBranches().GetByName(ctx, repo.ID, pull.HeadBranch)
				if err != nil && !database.IsErrBranchNotExist(err) {
					return nil, nil, nil, errors.Wrap(err, "get protected branch")
				}
				info.ViewerCanDeleteBranch = (protected == nil || !protected.Protected) && pull.HeadBranch != repo.DefaultBranch
			} else {
				// A protected branch keeps its protection after it is deleted.
				info.ViewerCanRestoreBranch, err = canPushTo(ctx, repoCtx, pull.HeadBranch, false)
				if err != nil {
					return nil, nil, nil, errors.Wrap(err, "check branch protection")
				}
			}
		}
		return info, rng, commits, nil
	}

	var headGitRepo *git.Repository
	if pull.HeadRepo != nil {
		if headGitRepo, err = git.Open(pull.HeadRepo.RepoPath()); err != nil {
			return nil, nil, nil, errors.Wrap(err, "open head repository")
		}
	}
	switch {
	case headGitRepo == nil || !headGitRepo.HasBranch(ctx, pull.HeadBranch):
		info.HeadLabel = "deleted"
		info.State = "broken"
	case !baseGitRepo.HasBranch(ctx, pull.BaseBranch):
		info.BaseLabel = "deleted"
		info.State = "broken"
	}
	if info.State == "broken" {
		if issue.IsClosed {
			info.State = "closed"
		}
		return info, nil, nil, nil
	}

	meta, err := headGitRepo.PullRequestMeta(ctx, database.RepoPath(repoCtx.Owner.Name, repo.Name), pull.HeadBranch, pull.BaseBranch)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "get pull request meta")
	}
	headCommitID, err := headGitRepo.BranchCommitID(ctx, pull.HeadBranch)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "get head commit")
	}
	info.CommitCount, info.FileCount = len(meta.Commits), meta.NumFiles
	rng = &pullRange{GitRepo: headGitRepo, Owner: pull.HeadUserName, Name: pull.HeadRepo.Name, Base: meta.MergeBase, Head: headCommitID}
	switch {
	case issue.IsClosed:
		info.State = "closed"
	case pull.IsChecking():
		info.State = "checking"
	case pull.CanAutoMerge():
		info.State = "mergeable"
		if repoCtx.Viewer.CanWrite() {
			info.ViewerCanMerge, err = canPushTo(ctx, repoCtx, pull.BaseBranch, true)
			if err != nil {
				return nil, nil, nil, errors.Wrap(err, "check push allowlist")
			}
			info.ViewerNotInAllowlist = !info.ViewerCanMerge
		}
		if info.ViewerCanMerge {
			info.ViewerID = repoCtx.Viewer.ID()
			pull.Issue = issue
			info.MergeMessages = map[database.MergeStyle]database.MergeMessage{}
			for _, style := range []database.MergeStyle{database.MergeStyleRegular, database.MergeStyleSquash} {
				if repo.AllowsMergeStyle(style) {
					info.MergeMessages[style] = pull.DefaultMergeMessage(style, meta.Commits)
				}
			}
		}
	default:
		info.State = "conflict"
	}
	return info, rng, meta.Commits, nil
}

// getRepoPull describes the pull request with its comments, and the state of
// its branches.
func getRepoPull(c flamego.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp *repoPullView, err error) {
	ctx := c.Request().Context()
	resp = &repoPullView{}
	var rng *pullRange
	if resp.Pull, rng, _, err = describePull(ctx, repoCtx, issue); err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if rng != nil {
		resp.Pull.Diff = &pullDiff{
			Base:               rng.Base,
			Head:               rng.Head,
			Owner:              rng.Owner,
			Name:               rng.Name,
			MarkdownExtensions: conf.Markdown.FileExtensions,
		}
	}
	if resp.repoIssueView, err = toRepoIssueView(ctx, logger, viewer, repoCtx, issue); err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if resp.Threads, resp.pullReviewStatus, err = describePullReviews(ctx, logger, viewer, repoCtx, issue, rng); err != nil {
		logger.Error("Failed to describe pull request reviews", "issueID", issue.ID, "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	// Only merges the reviews allow are offered.
	if len(resp.MergeBlockers) > 0 {
		resp.Pull.ViewerCanMerge = false
	}
	return http.StatusOK, resp, nil
}

// describePullReviews describes the threads of code comments the viewer can
// see in the commits of the pull request, and what its reviews decide. A
// broken pull request has no commits, so its threads are all outdated.
func describePullReviews(ctx context.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue, rng *pullRange) ([]issueComment, *pullReviewStatus, error) {
	cv := newCommentViewer(ctx, logger, viewer, repoCtx, issue)
	var gitRepo *git.Repository
	var base, head string
	if rng != nil {
		gitRepo, base, head = rng.GitRepo, rng.Base, rng.Head
	}
	threads, err := describeThreads(ctx, cv, gitRepo, base, head)
	if err != nil {
		return nil, nil, errors.Wrap(err, "describe threads")
	}
	status, err := describeReviews(ctx, logger, viewer, repoCtx, issue, head, threads)
	if err != nil {
		return nil, nil, errors.Wrap(err, "describe reviews")
	}
	return threads, status, nil
}

type repoPullCommits struct {
	Commits []repoCommitListItem `json:"commits"`
}

// getRepoPullCommits lists the commits the pull request merges, newest first.
func getRepoPullCommits(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp *repoPullCommits, err error) {
	ctx := c.Request().Context()
	_, rng, commits, err := describePull(ctx, repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if rng == nil {
		return http.StatusNotFound, nil, errors.New("branches of the pull request do not exist")
	}
	return http.StatusOK, &repoPullCommits{Commits: toCommitListItems(ctx, logger, repoCtx.Repo, commits)}, nil
}

// getRepoPullRaw writes the changes of the pull request as text.
func getRepoPullRaw(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, issue *database.Issue) {
	w := c.ResponseWriter()
	_, rng, _, err := describePull(c.Request().Context(), repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "repoID", repoCtx.Repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, err)
		return
	} else if rng == nil {
		writeErrorResponse(w, http.StatusNotFound, errors.New("branches of the pull request do not exist"))
		return
	}

	opt := git.RawDiffOptions{Base: rng.Base}
	if flag := whitespaceFlag(c.Request().URL.Query().Get("whitespace")); flag != "" {
		opt.Args = []string{flag}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err = rng.GitRepo.RawDiff(c.Request().Context(), rng.Head, git.RawDiffNormal, w, opt); err != nil {
		logger.Error("Failed to get pull request raw diff", "issueID", issue.ID, "error", err)
	}
}

type repoPullMergeRequest struct {
	// Style is "create_merge_commit", "rebase_before_merging" or
	// "squash_and_merge".
	Style string `json:"style" validate:"omitempty,oneof=create_merge_commit rebase_before_merging squash_and_merge"`
	// Title and Body are the message of the commit that merge commits and
	// squashing create. The default message is used when Title is empty.
	Title string `json:"title"`
	Body  string `json:"body"`
}

// canPushTo returns whether the protection of the branch lets the viewer, who
// must already be a writer, update it, which a merge of a pull request does
// when viaPullRequest is true. It takes the same rules as the pre-receive hook
// does for pushes.
func canPushTo(ctx context.Context, repoCtx *repoContext, branch string, viaPullRequest bool) (bool, error) {
	denial, err := database.Handle.ProtectBranches().CheckPushByName(ctx, repoCtx.Repo.ID, branch, repoCtx.Viewer.ID(), viaPullRequest)
	return denial == "", err
}

// postRepoPullMerge merges the pull request into its base branch.
func postRepoPullMerge(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue, req repoPullMergeRequest) (statusCode int, resp any, err error) {
	pull := issue.PullRequest
	if issue.IsClosed || pull.HasMerged || !pull.CanAutoMerge() {
		return http.StatusNotFound, nil, errors.New("pull request cannot be merged")
	}
	canPush, err := canPushTo(c.Request().Context(), repoCtx, pull.BaseBranch, true)
	if err != nil {
		logger.Error("Failed to check push allowlist", "pullID", pull.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "check push allowlist")
	} else if !canPush {
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("repo::pulls.merge_not_in_allowlist")}, nil
	}
	// Nobody merges past the reviews the base branch requires, admins
	// included. The merge takes the head commit the reviews were decided at.
	_, rng, _, err := describePull(c.Request().Context(), repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if rng == nil {
		return http.StatusNotFound, nil, errors.New("pull request cannot be merged")
	}
	if _, status, err := describePullReviews(c.Request().Context(), logger, viewer, repoCtx, issue, rng); err != nil {
		logger.Error("Failed to describe pull request reviews", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if len(status.MergeBlockers) > 0 {
		return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("repo::pulls.review.merge_blocked")}, nil
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if err = database.Handle.PullRequests().LoadAttributes(c.Request().Context(), pull); err != nil {
		logger.Error("Failed to load pull request attributes", "pullID", pull.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load attributes")
	}
	pull.Issue = issue
	pull.Issue.Repo = repoCtx.Repo
	style := database.MergeStyle(req.Style)
	if style == "" {
		style = database.MergeStyleRegular
	}
	if !repoCtx.Repo.AllowsMergeStyle(style) {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: l.Tr("repo::pulls.merge_style_not_allowed")}, nil
	}
	err = database.Handle.PullRequests().Merge(c.Request().Context(), logger, viewer, pull, gitRepo, rng.Head, style, database.MergeMessage{Title: req.Title, Body: req.Body})
	if errors.Is(err, database.ErrPullRequestNotMergeable) {
		return http.StatusNotFound, nil, errors.New("pull request cannot be merged")
	} else if errors.Is(err, database.ErrPullRequestHeadChanged) {
		return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("repo::pulls.review.head_changed")}, nil
	} else if err != nil {
		logger.Error("Failed to merge pull request", "pullID", pull.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "merge")
	}

	// The merge is done, so failing to delete the branch only leaves it be.
	if repoCtx.Repo.PullsDeleteBranch {
		info, _, _, err := describePull(c.Request().Context(), repoCtx, issue)
		if err != nil {
			logger.Error("Failed to describe pull request", "pullID", pull.ID, "error", err)
		} else if info.ViewerCanDeleteBranch {
			if _, err = deleteHeadBranch(c.Request().Context(), logger, viewer, repoCtx, pull); err != nil {
				logger.Error("Failed to delete head branch", "pullID", pull.ID, "error", err)
			}
		}
	}
	return http.StatusNoContent, nil, nil
}

// postRepoPullBranch restores the deleted head branch of the merged pull
// request at the commit that was merged, like GitHub's "Restore branch".
func postRepoPullBranch(c flamego.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	pull := issue.PullRequest
	info, _, _, err := describePull(ctx, repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if !info.ViewerCanRestoreBranch {
		return http.StatusNotFound, nil, errors.New("branch cannot be restored")
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if err = gitRepo.CreateBranch(ctx, pull.HeadBranch, pull.MergedCommitID); err != nil {
		logger.Error("Failed to create branch", "branch", pull.HeadBranch, "commitID", pull.MergedCommitID, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create branch")
	}
	// The branch exists now, so its webhook is queued even when the client has
	// gone away.
	if err = database.Handle.Webhooks().Prepare(context.WithoutCancel(ctx), logger, repo, database.HookEventTypeCreate, &v1types.WebhookCreatePayload{
		Ref:           pull.HeadBranch,
		RefType:       "branch",
		DefaultBranch: repo.DefaultBranch,
		Repo:          repo.APIFormatLegacy(logger, nil),
		Sender:        viewer.APIFormat(),
	}); err != nil {
		logger.Error("Failed to prepare webhooks for restored branch", "branch", pull.HeadBranch, "repoID", repo.ID, "error", err)
	}
	return http.StatusNoContent, nil, nil
}

// deleteRepoPullBranch deletes the head branch of the merged pull request,
// unless it has commits that were not merged.
func deleteRepoPullBranch(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	pull := issue.PullRequest
	info, _, _, err := describePull(ctx, repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if !info.ViewerCanDeleteBranch {
		return http.StatusNotFound, nil, errors.New("branch cannot be deleted")
	}
	deleted, err := deleteHeadBranch(ctx, logger, viewer, repoCtx, pull)
	if err != nil {
		logger.Error("Failed to delete head branch", "pullID", pull.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	} else if !deleted {
		return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("repo::pulls.delete_branch_has_new_commits")}, nil
	}
	return http.StatusNoContent, nil, nil
}

// deleteHeadBranch deletes the head branch of the merged pull request in the
// base repository, unless it has commits that were not merged, and returns
// whether it did.
func deleteHeadBranch(ctx context.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, pull *database.PullRequest) (deleted bool, err error) {
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		return false, errors.Wrap(err, "open repository")
	}
	commitID, err := gitRepo.BranchCommitID(ctx, pull.HeadBranch)
	if err != nil {
		return false, errors.Wrap(err, "get branch commit")
	} else if commitID != pull.MergedCommitID {
		return false, nil
	}
	if err = gitRepo.DeleteBranch(ctx, pull.HeadBranch, git.DeleteBranchOptions{Force: true}); err != nil {
		return false, errors.Wrap(err, "delete branch")
	}
	// The branch is gone now, so its webhook is queued even when the client has
	// gone away.
	if err = database.Handle.Webhooks().Prepare(context.WithoutCancel(ctx), logger, repo, database.HookEventTypeDelete, &v1types.WebhookDeletePayload{
		Ref:        pull.HeadBranch,
		RefType:    "branch",
		PusherType: v1types.WebhookPusherTypeUser,
		Repo:       repo.APIFormatLegacy(logger, nil),
		Sender:     viewer.APIFormat(),
	}); err != nil {
		logger.Error("Failed to prepare webhooks for deleted branch", "branch", pull.HeadBranch, "repoID", repo.ID, "error", err)
	}
	return true, nil
}
