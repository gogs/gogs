package web

import (
	"context"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// withRepoPulls lets through readers of a repository that takes pull
// requests. Like the server-rendered pages, they are not found otherwise.
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
func withRepoPull(c flamego.Context, repoCtx *repoContext) {
	injectRepoIssue(c, repoCtx, true)
}

type repoPullInfo struct {
	// State is "merged", "closed", "broken" when a branch or the fork is
	// gone, "checking" while conflicts are checked, "mergeable" or
	// "conflict".
	State      string `json:"state"`
	HeadBranch string `json:"headBranch"`
	BaseBranch string `json:"baseBranch"`
	// HeadLabel and BaseLabel name the branches after their owners, or are
	// "deleted" for a branch that is gone, like the server-rendered page.
	HeadLabel string `json:"headLabel"`
	BaseLabel string `json:"baseLabel"`
	// HeadRepoLink is the repository of the head branch, unless it is gone.
	HeadRepoLink string     `json:"headRepoLink,omitempty"`
	CommitCount  int        `json:"commitCount"`
	FileCount    int        `json:"fileCount"`
	Merger       *issueUser `json:"merger,omitempty"`
	MergedAt     *time.Time `json:"mergedAt,omitempty"`
	AllowRebase  bool       `json:"allowRebase"`
	// ViewerCanMerge is whether the viewer can merge it now.
	ViewerCanMerge bool `json:"viewerCanMerge"`
	// ViewerCanDeleteBranch is whether the viewer can delete the head branch
	// after the merge: a branch of the same repository that is not protected
	// and has no new commits.
	ViewerCanDeleteBranch bool `json:"viewerCanDeleteBranch"`
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
}

type repoPullView struct {
	*repoIssueView
	Pull *repoPullInfo `json:"pull"`
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
	if err = pull.LoadAttributes(); err != nil {
		return nil, nil, nil, errors.Wrap(err, "load attributes")
	}
	info = &repoPullInfo{
		HeadBranch:  pull.HeadBranch,
		BaseBranch:  pull.BaseBranch,
		HeadLabel:   pull.HeadUserName + "/" + pull.HeadBranch,
		BaseLabel:   repoCtx.Owner.Name + "/" + pull.BaseBranch,
		AllowRebase: repo.PullsAllowRebase,
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

		if repoCtx.Viewer.CanWrite() && pull.HeadRepoID == pull.BaseRepoID && baseGitRepo.HasBranch(ctx, pull.HeadBranch) {
			protected, err := database.GetProtectBranchOfRepoByName(repo.ID, pull.HeadBranch)
			if err != nil && !database.IsErrBranchNotExist(err) {
				return nil, nil, nil, errors.Wrap(err, "get protected branch")
			}
			info.ViewerCanDeleteBranch = (protected == nil || !protected.Protected) && pull.HeadBranch != repo.DefaultBranch
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
		info.ViewerCanMerge = repoCtx.Viewer.CanWrite()
	default:
		info.State = "conflict"
	}
	return info, rng, meta.Commits, nil
}

// getRepoPull describes the pull request with its comments, and the state of
// its branches.
func getRepoPull(c flamego.Context, user *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp *repoPullView, err error) {
	ctx := c.Request().Context()
	resp = &repoPullView{}
	var rng *pullRange
	if resp.Pull, rng, _, err = describePull(ctx, repoCtx, issue); err != nil {
		log.Error("getRepoPull: describe pull request %d of repository %d: %v", issue.ID, repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	if rng != nil {
		resp.Pull.Diff = &pullDiff{Base: rng.Base, Head: rng.Head, Owner: rng.Owner, Name: rng.Name}
	}
	if resp.repoIssueView, err = toRepoIssueView(ctx, user, repoCtx, issue); err != nil {
		log.Error("getRepoPull: describe pull request %d of repository %d: %v", issue.ID, repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, resp, nil
}

type repoPullCommits struct {
	Commits []repoCommitListItem `json:"commits"`
}

// getRepoPullCommits lists the commits the pull request merges, newest first.
func getRepoPullCommits(c flamego.Context, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp *repoPullCommits, err error) {
	ctx := c.Request().Context()
	_, rng, commits, err := describePull(ctx, repoCtx, issue)
	if err != nil {
		log.Error("getRepoPullCommits: describe pull request %d of repository %d: %v", issue.ID, repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, err
	} else if rng == nil {
		return http.StatusNotFound, nil, errors.New("branches of the pull request do not exist")
	}
	return http.StatusOK, &repoPullCommits{Commits: toCommitListItems(ctx, repoCtx.Repo, commits)}, nil
}

// getRepoPullRaw writes the changes of the pull request as text.
func getRepoPullRaw(c flamego.Context, repoCtx *repoContext, issue *database.Issue) {
	w := c.ResponseWriter()
	_, rng, _, err := describePull(c.Request().Context(), repoCtx, issue)
	if err != nil {
		log.Error("getRepoPullRaw: describe pull request %d of repository %d: %v", issue.ID, repoCtx.Repo.ID, err)
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
		log.Error("getRepoPullRaw: get raw diff of pull request %d: %v", issue.ID, err)
	}
}

type repoPullMergeRequest struct {
	// Style is "create_merge_commit" or "rebase_before_merging".
	Style       string `json:"style" validate:"omitempty,oneof=create_merge_commit rebase_before_merging"`
	Description string `json:"description"`
}

// postRepoPullMerge merges the pull request into its base branch.
func postRepoPullMerge(c flamego.Context, user *database.User, repoCtx *repoContext, issue *database.Issue, req repoPullMergeRequest) (statusCode int, resp any, err error) {
	pull := issue.PullRequest
	if issue.IsClosed || pull.HasMerged || !pull.CanAutoMerge() {
		return http.StatusNotFound, nil, errors.New("pull request cannot be merged")
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("postRepoPullMerge: open repository %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if err = pull.LoadAttributes(); err != nil {
		log.Error("postRepoPullMerge: load attributes of pull request %d: %v", pull.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load attributes")
	}
	pull.Issue = issue
	pull.Issue.Repo = repoCtx.Repo
	style := database.MergeStyle(req.Style)
	if style == "" {
		style = database.MergeStyleRegular
	}
	if err = pull.Merge(c.Request().Context(), user, gitRepo, style, req.Description); err != nil {
		log.Error("postRepoPullMerge: merge pull request %d: %v", pull.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "merge")
	}
	return http.StatusNoContent, nil, nil
}

// deleteRepoPullBranch deletes the head branch of the merged pull request,
// unless it has commits that were not merged.
func deleteRepoPullBranch(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	pull := issue.PullRequest
	info, _, _, err := describePull(ctx, repoCtx, issue)
	if err != nil {
		log.Error("deleteRepoPullBranch: describe pull request %d of repository %d: %v", issue.ID, repo.ID, err)
		return http.StatusInternalServerError, nil, err
	} else if !info.ViewerCanDeleteBranch {
		return http.StatusNotFound, nil, errors.New("branch cannot be deleted")
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("deleteRepoPullBranch: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	commitID, err := gitRepo.BranchCommitID(ctx, pull.HeadBranch)
	if err != nil {
		log.Error("deleteRepoPullBranch: get commit of branch %q in repository %d: %v", pull.HeadBranch, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	} else if commitID != pull.MergedCommitID {
		return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("repo.pulls.delete_branch_has_new_commits")}, nil
	}
	if err = gitRepo.DeleteBranch(ctx, pull.HeadBranch, git.DeleteBranchOptions{Force: true}); err != nil {
		log.Error("deleteRepoPullBranch: delete branch %q of repository %d: %v", pull.HeadBranch, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete branch")
	}
	if err = database.PrepareWebhooks(repo, database.HookEventTypeDelete, &apiv1types.WebhookDeletePayload{
		Ref:        pull.HeadBranch,
		RefType:    "branch",
		PusherType: apiv1types.WebhookPusherTypeUser,
		Repo:       repo.APIFormatLegacy(nil),
		Sender:     user.APIFormat(),
	}); err != nil {
		log.Error("deleteRepoPullBranch: prepare webhooks for deleting branch %q of repository %d: %v", pull.HeadBranch, repo.ID, err)
	}
	return http.StatusNoContent, nil, nil
}
