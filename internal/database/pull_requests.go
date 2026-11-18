package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/osx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/sync"
)

var PullRequestQueue = sync.NewUniqueQueue(1000)

type PullRequestType int

const (
	PullRequestTypeGogs PullRequestType = iota
	PullRequestTypeGit
)

type PullRequestStatus int

const (
	PullRequestStatusConflict PullRequestStatus = iota
	PullRequestStatusChecking
	PullRequestStatusMergeable
)

// PullRequest represents relation between pull request and repositories.
type PullRequest struct {
	ID     int64 `gorm:"primaryKey"`
	Type   PullRequestType
	Status PullRequestStatus

	IssueID int64  `xorm:"INDEX" gorm:"index"`
	Issue   *Issue `xorm:"-" json:"-" gorm:"-"`
	Index   int64

	HeadRepoID   int64
	HeadRepo     *Repository `xorm:"-" json:"-" gorm:"-"`
	BaseRepoID   int64
	BaseRepo     *Repository `xorm:"-" json:"-" gorm:"-"`
	HeadUserName string
	HeadBranch   string
	BaseBranch   string
	MergeBase    string `xorm:"VARCHAR(64)" gorm:"type:VARCHAR(64)"`

	HasMerged      bool
	MergedCommitID string `xorm:"VARCHAR(64)" gorm:"type:VARCHAR(64)"`
	MergerID       int64
	Merger         *User     `xorm:"-" json:"-" gorm:"-"`
	Merged         time.Time `xorm:"-" json:"-" gorm:"-"`
	MergedUnix     int64
}

// BeforeUpdate implements the GORM update hook.
func (pr *PullRequest) BeforeUpdate(_ *gorm.DB) error {
	pr.MergedUnix = pr.Merged.Unix()
	return nil
}

// AfterFind implements the GORM query hook.
//
// NOTE: Don't try to get Issue because it will end up in recursive querying.
func (pr *PullRequest) AfterFind(_ *gorm.DB) error {
	if pr.HasMerged {
		pr.Merged = time.Unix(pr.MergedUnix, 0).Local()
	}
	return nil
}

// APIFormat returns the API format of the pull request. It assumes the
// following fields have been assigned with valid values:
// Required - Issue, BaseRepo
// Optional - HeadRepo, Merger
func (pr *PullRequest) APIFormat(logger *logx.Logger) *apiv1types.PullRequest {
	// In case the head repo has been deleted.
	var apiHeadRepo *apiv1types.Repository
	if pr.HeadRepo == nil {
		apiHeadRepo = &apiv1types.Repository{
			Name: "deleted",
		}
	} else {
		apiHeadRepo = pr.HeadRepo.APIFormatLegacy(logger, nil)
	}

	apiIssue := pr.Issue.APIFormat()
	apiPullRequest := &apiv1types.PullRequest{
		ID:         pr.ID,
		Index:      pr.Index,
		Poster:     apiIssue.Poster,
		Title:      apiIssue.Title,
		Body:       apiIssue.Body,
		Labels:     apiIssue.Labels,
		Milestone:  apiIssue.Milestone,
		Assignee:   apiIssue.Assignee,
		State:      apiIssue.State,
		Comments:   apiIssue.Comments,
		HeadBranch: pr.HeadBranch,
		HeadRepo:   apiHeadRepo,
		BaseBranch: pr.BaseBranch,
		BaseRepo:   pr.BaseRepo.APIFormatLegacy(logger, nil),
		HTMLURL:    pr.Issue.HTMLURL(),
		HasMerged:  pr.HasMerged,
	}

	if pr.Status != PullRequestStatusChecking {
		mergeable := pr.Status != PullRequestStatusConflict
		apiPullRequest.Mergeable = &mergeable
	}
	if pr.HasMerged {
		apiPullRequest.Merged = &pr.Merged
		apiPullRequest.MergedCommitID = &pr.MergedCommitID
		apiPullRequest.MergedBy = pr.Merger.APIFormat()
	}

	return apiPullRequest
}

// IsChecking returns true if this pull request is still checking conflict.
func (pr *PullRequest) IsChecking() bool {
	return pr.Status == PullRequestStatusChecking
}

// CanAutoMerge returns true if this pull request can be merged automatically.
func (pr *PullRequest) CanAutoMerge() bool {
	return pr.Status == PullRequestStatusMergeable
}

// MergeStyle represents the approach to merge commits into base branch.
type MergeStyle string

const (
	MergeStyleRegular MergeStyle = "create_merge_commit"
	MergeStyleRebase  MergeStyle = "rebase_before_merging"
	MergeStyleSquash  MergeStyle = "squash_and_merge"
)

// AllowsMergeStyle returns whether pull requests of the repository can be
// merged with the style.
func (repo *Repository) AllowsMergeStyle(style MergeStyle) bool {
	switch style {
	case MergeStyleRegular:
		return repo.PullsAllowMerge
	case MergeStyleRebase:
		return repo.PullsAllowRebase
	case MergeStyleSquash:
		return repo.PullsAllowSquash
	default:
		return false
	}
}

// The presets of the messages merge commits and squashing start with. The
// empty preset is the default message, like on GitHub: "Merge pull request #N
// from <head>" for merge commits, and for squashing, the message of the commit
// when there is one, or the title of the pull request with the subjects of
// its commits.
const (
	MergeMessageDefault          = ""
	MergeMessageTitle            = "title"
	MergeMessageTitleDescription = "title_description"
	// MergeMessageTitleCommits is only for squashing.
	MergeMessageTitleCommits = "title_commits"
)

// MergeMessage is the title and body of the commit that merges a pull
// request.
type MergeMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// DefaultMergeMessage returns the message the commit of the merge style starts
// with, before the merger edits it, following the preset of the base
// repository. The commits are those the pull request merges, newest first.
// Rebasing keeps the messages of the commits, so it has none.
func (pr *PullRequest) DefaultMergeMessage(mergeStyle MergeStyle, commits []*git.Commit) MergeMessage {
	message := pr.presetMergeMessage(mergeStyle, commits)
	// Squashing leaves only the author of the pull request on the commit, so
	// the other authors are credited the way GitHub does. The trailers must
	// end the message to count, so those the body copies from commit messages
	// or the description move there, and each person is credited once.
	if mergeStyle == MergeStyleSquash {
		var credited []string
		message.Body, credited = cutCoAuthorLines(message.Body)
		if trailers := coAuthorTrailers(commits, credited, pr.Issue.Poster); trailers != "" {
			if message.Body != "" {
				message.Body += "\n\n"
			}
			message.Body += trailers
		}
	}
	return message
}

// parseCoAuthorLine returns the name and email of a "Co-authored-by" trailer
// line, matched case-insensitively.
func parseCoAuthorLine(line string) (name, email string, ok bool) {
	const prefix = "co-authored-by:"
	line = strings.TrimSpace(line)
	if len(line) <= len(prefix) || !strings.EqualFold(line[:len(prefix)], prefix) {
		return "", "", false
	}
	name, email, ok = strings.Cut(line[len(prefix):], "<")
	email, closed := strings.CutSuffix(email, ">")
	return name, email, ok && closed
}

// cutCoAuthorLines removes the "Co-authored-by" lines from the text, and
// returns the text left and the lines removed.
func cutCoAuthorLines(text string) (rest string, lines []string) {
	var kept strings.Builder
	for line := range strings.Lines(text) {
		if _, _, ok := parseCoAuthorLine(line); ok {
			lines = append(lines, line)
			continue
		}
		kept.WriteString(line)
	}
	if lines == nil {
		return text, nil
	}
	// Removing the lines can leave runs of blank lines behind.
	return strings.TrimSpace(blankLines.ReplaceAllString(kept.String(), "\n\n")), lines
}

var blankLines = lazyregexp.New(`\n(?:[ \t]*\n){2,}`)

// coAuthorTrailers returns a "Co-authored-by" trailer for each author of the
// commits, oldest first, each co-author their messages already credit, and
// then each co-author of the credited lines, except the poster of the pull
// request.
func coAuthorTrailers(commits []*git.Commit, credited []string, poster *User) string {
	seen := map[string]bool{}
	if poster != nil && poster.ID > 0 {
		seen[strings.ToLower(poster.Email)] = true
	}
	var trailers []string
	add := func(name, email string) {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		trailers = append(trailers, fmt.Sprintf("Co-authored-by: %s <%s>", strings.TrimSpace(name), strings.TrimSpace(email)))
	}
	addLine := func(line string) {
		if name, email, ok := parseCoAuthorLine(line); ok {
			add(name, email)
		}
	}
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		if c.Author != nil {
			add(c.Author.Name, c.Author.Email)
		}
		for line := range strings.Lines(c.Message) {
			addLine(line)
		}
	}
	for _, line := range credited {
		addLine(line)
	}
	return strings.Join(trailers, "\n")
}

// presetMergeMessage returns the message of the preset of the base repository
// for the merge style.
func (pr *PullRequest) presetMergeMessage(mergeStyle MergeStyle, commits []*git.Commit) MergeMessage {
	titled := fmt.Sprintf("%s (#%d)", pr.Issue.Title, pr.Index)
	switch mergeStyle {
	case MergeStyleRegular:
		switch pr.BaseRepo.PullsMergeMessage {
		case MergeMessageTitle:
			return MergeMessage{Title: titled}
		case MergeMessageTitleDescription:
			return MergeMessage{Title: titled, Body: pr.Issue.Content}
		}
		return MergeMessage{
			Title: fmt.Sprintf("Merge pull request #%d from %s/%s", pr.Index, pr.HeadUserName, pr.HeadBranch),
			Body:  pr.Issue.Title,
		}

	case MergeStyleSquash:
		switch pr.BaseRepo.PullsSquashMessage {
		case MergeMessageTitle:
			return MergeMessage{Title: titled}
		case MergeMessageTitleDescription:
			return MergeMessage{Title: titled, Body: pr.Issue.Content}
		case MergeMessageTitleCommits:
			return MergeMessage{Title: titled, Body: commitList(commits, true)}
		}
		if len(commits) == 1 {
			title, body, _ := strings.Cut(strings.TrimSpace(commits[0].Message), "\n")
			return MergeMessage{Title: fmt.Sprintf("%s (#%d)", title, pr.Index), Body: strings.TrimSpace(body)}
		}
		return MergeMessage{Title: titled, Body: commitList(commits, false)}
	}
	return MergeMessage{}
}

// commitList lists the subjects of the commits, oldest first, with their
// bodies when withBodies is true.
func commitList(commits []*git.Commit, withBodies bool) string {
	items := make([]string, 0, len(commits))
	for i := len(commits) - 1; i >= 0; i-- {
		subject, body, _ := strings.Cut(strings.TrimSpace(commits[i].Message), "\n")
		item := "* " + subject
		if body = strings.TrimSpace(body); withBodies && body != "" {
			item += "\n\n" + body
		}
		items = append(items, item)
	}
	sep := "\n"
	if withBodies {
		sep = "\n\n"
	}
	return strings.Join(items, sep)
}

// ErrPullRequestNotMergeable is returned by Merge when the pull request is
// merged or closed by someone else first.
var ErrPullRequestNotMergeable = errors.New("pull request has been merged or closed")

// Merge merges pull request to base repository. The message is used for the
// commit that merge commits and squashing create, and its default is used
// when its title is empty. It returns ErrPullRequestNotMergeable when the
// pull request is merged or closed by someone else first.
func (s *PullRequestsStore) Merge(ctx context.Context, logger *logx.Logger, doer *User, pr *PullRequest, baseGitRepo *git.Repository, mergeStyle MergeStyle, message MergeMessage) (err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	defer func() {
		go HookQueue.Add(pr.BaseRepo.ID)
		go AddTestPullRequestTask(logger, doer, pr.BaseRepo.ID, pr.BaseBranch, false)
	}()

	tx := s.db.WithContext(ctx).Begin()
	if err = tx.Error; err != nil {
		return err
	}
	defer tx.Rollback()

	// Claim the pull request first. This locks its row, so a concurrent merge
	// waits here and then finds it merged.
	result := tx.Model(&PullRequest{}).Where("id = ? AND has_merged = ?", pr.ID, false).UpdateColumn("has_merged", true)
	if result.Error != nil {
		return errors.Wrap(result.Error, "claim pull request")
	} else if result.RowsAffected == 0 {
		return ErrPullRequestNotMergeable
	}

	err = newIssuesStore(tx).changeStatus(ctx, logger, doer, pr.Issue.Repo, pr.Issue, true)
	if errors.Is(err, errIssueStatusUnchanged) {
		return ErrPullRequestNotMergeable
	} else if err != nil {
		return errors.Wrap(err, "close issue")
	}

	headRepoPath := RepoPath(pr.HeadUserName, pr.HeadRepo.Name)
	headGitRepo, err := git.Open(headRepoPath)
	if err != nil {
		return errors.Newf("open repository: %v", err)
	}

	baseCommitID, err := baseGitRepo.BranchCommitID(ctx, pr.BaseBranch)
	if err != nil {
		return errors.Newf("get base branch %q commit ID: %v", pr.BaseBranch, err)
	}
	headCommitID, err := headGitRepo.BranchCommitID(ctx, pr.HeadBranch)
	if err != nil {
		return errors.Newf("get head branch %q commit ID: %v", pr.HeadBranch, err)
	}

	// The temporary repository reads objects of both repositories, so the merge
	// result can be built from them without cloning either.
	tmpRepo, cleanup, err := newTemporaryRepository(ctx, baseGitRepo.Path(), headRepoPath)
	if err != nil {
		return errors.Newf("create temporary repository: %v", err)
	}
	defer cleanup()

	if !pr.BaseRepo.AllowsMergeStyle(mergeStyle) {
		return errors.Newf("merge style %q is not allowed", mergeStyle)
	}

	if strings.TrimSpace(message.Title) == "" {
		commits, err := tmpRepo.RevList(ctx, []string{baseCommitID + ".." + headCommitID})
		if err != nil {
			return errors.Newf("list commits: %v", err)
		}
		message = pr.DefaultMergeMessage(mergeStyle, commits)
	}
	commitMessage := git.CleanupMessage(message.Title + "\n\n" + message.Body)

	committer := &git.Signature{Name: doer.DisplayName(), Email: doer.Email}
	var mergedCommitID string
	switch mergeStyle {
	case MergeStyleRegular: // Create merge commit.
		// There is nothing to merge when the base branch already has the head branch.
		merged, err := tmpRepo.IsAncestor(ctx, headCommitID, baseCommitID)
		if err != nil {
			return errors.Newf("check if head branch is merged: %v", err)
		} else if merged {
			mergedCommitID = baseCommitID
			break
		}

		tree, err := tmpRepo.MergeTree(ctx, baseCommitID, headCommitID)
		if err != nil {
			return errors.Newf("merge tree: %v", err)
		}

		mergedCommitID, err = tmpRepo.CommitTree(ctx, tree, commitMessage, git.CommitTreeOptions{
			Parents:   []string{baseCommitID, headCommitID},
			Author:    committer,
			Committer: committer,
		})
		if err != nil {
			return errors.Newf("commit tree: %v", err)
		}

	case MergeStyleRebase: // Rebase before merging.
		mergedCommitID, err = tmpRepo.Rebase(ctx, baseCommitID, headCommitID, committer)
		if err != nil {
			return errors.Newf("rebase [%s on %s]: %v", pr.HeadBranch, pr.BaseBranch, err)
		}

	case MergeStyleSquash: // Squash all changes into one commit on top of the base branch.
		merged, err := tmpRepo.IsAncestor(ctx, headCommitID, baseCommitID)
		if err != nil {
			return errors.Newf("check if head branch is merged: %v", err)
		} else if merged {
			mergedCommitID = baseCommitID
			break
		}

		tree, err := tmpRepo.MergeTree(ctx, baseCommitID, headCommitID)
		if err != nil {
			return errors.Newf("merge tree: %v", err)
		}

		// The squashed changes are attributed to the author of the pull request,
		// as on GitHub, unless that account no longer exists.
		author := committer
		if pr.Issue.Poster != nil && pr.Issue.Poster.ID > 0 {
			author = &git.Signature{Name: pr.Issue.Poster.DisplayName(), Email: pr.Issue.Poster.Email}
		}
		mergedCommitID, err = tmpRepo.CommitTree(ctx, tree, commitMessage, git.CommitTreeOptions{
			Parents:   []string{baseCommitID},
			Author:    author,
			Committer: committer,
		})
		if err != nil {
			return errors.Newf("commit tree: %v", err)
		}

	default:
		return errors.Newf("unknown merge style: %s", mergeStyle)
	}

	// Push changes on base branch to upstream.
	if err = tmpRepo.Push(ctx, baseGitRepo.Path(), mergedCommitID+":"+git.RefsHeads+pr.BaseBranch); err != nil {
		return errors.Newf("git push: %v", err)
	}

	pr.MergedCommitID, err = headGitRepo.BranchCommitID(ctx, pr.HeadBranch)
	if err != nil {
		return errors.Newf("get head branch %q commit ID: %v", pr.HeadBranch, err)
	}

	pr.HasMerged = true
	pr.Merged = time.Now()
	pr.MergerID = doer.ID
	if err = tx.Model(pr).Select("*").Updates(pr).Error; err != nil {
		return errors.Newf("update pull request: %v", err)
	}

	if err = tx.Commit().Error; err != nil {
		return errors.Newf("commit: %v", err)
	}

	if err = newActionsStore(s.db).MergePullRequest(ctx, doer, pr.Issue.Repo.Owner, pr.Issue.Repo, pr.Issue); err != nil {
		logger.Error("Failed to create action for merging pull request", "pullRequestID", pr.ID, "error", err)
	}

	// Reload pull request information.
	if err = s.LoadAttributes(ctx, pr); err != nil {
		logger.Error("Failed to load pull request attributes", "pullRequestID", pr.ID, "error", err)
		return nil
	}
	if err = newWebhooksStore(s.db).Prepare(ctx, logger, pr.Issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
		Action:      apiv1types.WebhookIssueClosed,
		Index:       pr.Index,
		PullRequest: pr.APIFormat(logger),
		Repository:  pr.Issue.Repo.APIFormatLegacy(logger, nil),
		Sender:      doer.APIFormat(),
	}); err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
		return nil
	}

	commits, err := headGitRepo.RevList(ctx, []string{pr.MergeBase + "..." + pr.MergedCommitID})
	if err != nil {
		logger.Error("Failed to list commits", "mergeBase", pr.MergeBase, "mergedCommitID", pr.MergedCommitID, "error", err)
		return nil
	}

	// NOTE: It is possible that head branch is not fully synced with base branch
	// for merge commits, so we need to get the latest commit of the base branch
	// and add it as the merge commit manually to avoid strange diff commits
	// produced.
	mergeCommit, err := baseGitRepo.BranchCommit(ctx, pr.BaseBranch)
	if err != nil {
		logger.Error("Failed to get base branch commit", "baseBranch", pr.BaseBranch, "error", err)
		return nil
	}
	switch mergeStyle {
	case MergeStyleRegular:
		commits = append([]*git.Commit{mergeCommit}, commits...)
	case MergeStyleSquash:
		// Only the squashed commit lands on the base branch.
		commits = []*git.Commit{mergeCommit}
	}

	pcs, err := CommitsToPushCommits(commits).APIFormat(ctx, newUsersStore(s.db), pr.BaseRepo.RepoPath(), pr.BaseRepo.HTMLURL())
	if err != nil {
		logger.Error("Failed to convert to API payload commits", "error", err)
		return nil
	}

	p := &apiv1types.WebhookPushPayload{
		Ref:        git.RefsHeads + pr.BaseBranch,
		Before:     pr.MergeBase,
		After:      mergeCommit.ID.String(),
		CompareURL: conf.Server.ExternalURL + pr.BaseRepo.ComposeCompareURL(pr.MergeBase, pr.MergedCommitID),
		Commits:    pcs,
		Repo:       pr.BaseRepo.APIFormatLegacy(logger, nil),
		Pusher:     pr.HeadRepo.MustOwner().APIFormat(),
		Sender:     doer.APIFormat(),
	}
	if err = newWebhooksStore(s.db).Prepare(ctx, logger, pr.BaseRepo, HookEventTypePush, p); err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
		return nil
	}
	return nil
}

// testPatch checks if the patch of the pull request can be applied to its
// base branch without conflicts.
func (s *PullRequestsStore) testPatch(ctx context.Context, logger *logx.Logger, pr *PullRequest) (err error) {
	if pr.BaseRepo == nil {
		pr.BaseRepo, err = getRepositoryByID(s.db.WithContext(ctx), pr.BaseRepoID)
		if err != nil {
			return errors.Wrap(err, "get base repository")
		}
	}

	patchPath, err := pr.BaseRepo.PatchPath(pr.Index)
	if err != nil {
		return errors.Newf("BaseRepo.PatchPath: %v", err)
	}

	// Fast fail if patch does not exist, which means data is corrupted.
	if !osx.IsFile(patchPath) {
		logger.Debug("Skipped testing patch because the patch file does not exist", "pullRequestID", pr.ID)
		return nil
	}

	logger.Debug("Testing patch", "pullRequestID", pr.ID, "patchPath", patchPath)

	baseCommitID, err := git.RepoBranchCommitID(ctx, pr.BaseRepo.RepoPath(), pr.BaseBranch)
	if err != nil {
		return errors.Newf("get base branch %q commit ID: %v", pr.BaseBranch, err)
	}

	tmpRepo, cleanup, err := newTemporaryRepository(ctx, pr.BaseRepo.RepoPath())
	if err != nil {
		return errors.Newf("create temporary repository: %v", err)
	}
	defer cleanup()
	if err = tmpRepo.ReadTree(ctx, baseCommitID); err != nil {
		return errors.Newf("read tree: %v", err)
	}

	pr.Status = PullRequestStatusChecking
	err = tmpRepo.ApplyCheck(ctx, patchPath, git.ApplyCheckOptions{
		IgnoreWhitespace: pr.BaseRepo.PullsIgnoreWhitespace,
	})
	if err != nil {
		logger.Debug("Patch has conflicts", "pullRequestID", pr.ID, "error", err)
		pr.Status = PullRequestStatusConflict
		return nil
	}
	return nil
}

var _ errx.NotFound = (*ErrPullRequestNotExist)(nil)

type ErrPullRequestNotExist struct {
	args map[string]any
}

func IsErrPullRequestNotExist(err error) bool {
	_, ok := err.(ErrPullRequestNotExist)
	return ok
}

func (err ErrPullRequestNotExist) Error() string {
	return fmt.Sprintf("pull request does not exist: %v", err.args)
}

func (ErrPullRequestNotExist) NotFound() bool {
	return true
}

// PushToBaseRepo pushes commits from branches of head repository to
// corresponding branches of base repository.
// FIXME: Only push branches that are actually updates?
func (pr *PullRequest) PushToBaseRepo(ctx context.Context, logger *logx.Logger) (err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	logger.Debug("Pushing commits to base repository", "baseRepoID", pr.BaseRepoID, "index", pr.Index)

	headRepoPath := pr.HeadRepo.RepoPath()
	headGitRepo, err := git.Open(headRepoPath)
	if err != nil {
		return errors.Newf("open repository: %v", err)
	}

	tmpRemote := fmt.Sprintf("tmp-pull-%d", pr.ID)
	if err = headGitRepo.RemoteAdd(ctx, tmpRemote, pr.BaseRepo.RepoPath()); err != nil {
		return errors.Newf("add remote %q [repo_id: %d]: %v", tmpRemote, pr.HeadRepoID, err)
	}

	// Make sure to remove the remote even if the push fails.
	defer func() {
		if err := headGitRepo.RemoteRemove(ctx, tmpRemote); err != nil {
			logger.Error("Failed to remove temporary remote", "remote", tmpRemote, "repoID", pr.HeadRepoID, "error", err)
		}
	}()

	headRefspec := fmt.Sprintf("refs/pull/%d/head", pr.Index)
	headFile := filepath.Join(pr.BaseRepo.RepoPath(), headRefspec)
	if osx.Exist(headFile) {
		err = os.Remove(headFile)
		if err != nil {
			return errors.Newf("remove head file [repo_id: %d]: %v", pr.BaseRepoID, err)
		}
	}

	err = headGitRepo.Push(ctx, tmpRemote, fmt.Sprintf("%s:%s", pr.HeadBranch, headRefspec))
	if err != nil {
		return errors.Newf("push: %v", err)
	}

	return nil
}

// AddTestPullRequestTask queues tests of the pull requests from and into the
// branch of the repository, updating the patches of those from it. When isSync
// is true, the ones from it are announced as synchronized.
func AddTestPullRequestTask(logger *logx.Logger, doer *User, repoID int64, branch string, isSync bool) {
	ctx := context.Background()
	pulls := Handle.PullRequests()
	logger.Debug("Finding pull requests by head branch", "headRepoID", repoID, "headBranch", branch)
	prs, err := pulls.listUnmergedByHead(ctx, repoID, branch)
	if err != nil {
		logger.Error("Failed to find pull requests", "headRepoID", repoID, "headBranch", branch, "error", err)
		return
	}

	if isSync {
		issues := Handle.Issues()
		for _, pr := range prs {
			if err = pulls.LoadAttributes(ctx, pr); err != nil {
				logger.Error("Failed to load pull request attributes", "pullRequestID", pr.ID, "error", err)
				continue
			}
			pr.Issue, err = issues.getRaw(ctx, map[string]any{"issueID": pr.IssueID}, "id = ?", pr.IssueID)
			if err == nil {
				pr.Issue.PullRequest = pr
				err = issues.LoadAttributes(ctx, pr.Issue)
			}
			if err != nil {
				logger.Error("Failed to load issue", "pullRequestID", pr.ID, "error", err)
				continue
			}
			err = Handle.Webhooks().Prepare(ctx, logger, pr.Issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
				Action:      apiv1types.WebhookIssueSynchronized,
				Index:       pr.Issue.Index,
				PullRequest: pr.APIFormat(logger),
				Repository:  pr.Issue.Repo.APIFormatLegacy(logger, nil),
				Sender:      doer.APIFormat(),
			})
			if err != nil {
				logger.Error("Failed to prepare webhooks", "pullRequestID", pr.ID, "error", err)
			}
		}
	}

	pulls.addHeadRepoTasks(ctx, logger, prs)

	logger.Debug("Finding pull requests by base branch", "baseRepoID", repoID, "baseBranch", branch)
	prs, err = pulls.listUnmergedByBase(ctx, repoID, branch)
	if err != nil {
		logger.Error("Failed to find pull requests", "baseRepoID", repoID, "baseBranch", branch, "error", err)
		return
	}
	for _, pr := range prs {
		pulls.AddToTaskQueue(ctx, logger, pr)
	}
}

// TestPullRequests tests the patches of pull requests that are being checked,
// then those queued to be tested as they come.
//
// TODO: test more pull requests at same time.
func TestPullRequests(logger *logx.Logger) {
	ctx := context.Background()
	pulls := Handle.PullRequests()

	var checking []*PullRequest
	if err := pulls.db.WithContext(ctx).Where("status = ?", PullRequestStatusChecking).Find(&checking).Error; err != nil {
		logger.Error("Failed to find pull requests to test", "error", err)
	}
	prs := make([]*PullRequest, 0, len(checking))
	for _, pr := range checking {
		if err := pulls.LoadAttributes(ctx, pr); err != nil {
			logger.Error("Failed to load pull request attributes", "error", err)
			continue
		}

		if err := pulls.testPatch(ctx, logger, pr); err != nil {
			logger.Error("Failed to test patch", "error", err)
			continue
		}
		prs = append(prs, pr)
	}

	for _, pr := range prs {
		pulls.checkAndUpdateStatus(ctx, logger, pr)
	}

	for prID := range PullRequestQueue.Queue() {
		logger.Debug("Processing test task", "pullRequestID", prID)
		PullRequestQueue.Remove(prID)

		id, _ := strconv.ParseInt(prID, 10, 64)
		pr, err := pulls.GetByID(ctx, id)
		if err != nil {
			logger.Error("Failed to get pull request", "pullRequestID", prID, "error", err)
			continue
		} else if err = pulls.testPatch(ctx, logger, pr); err != nil {
			logger.Error("Failed to test patch", "pullRequestID", pr.ID, "error", err)
			continue
		}

		pulls.checkAndUpdateStatus(ctx, logger, pr)
	}
}

func InitTestPullRequests(logger *logx.Logger) {
	go TestPullRequests(logger)
}

// PullRequestsStore is the storage layer for pull requests.
type PullRequestsStore struct {
	db *gorm.DB
}

func newPullRequestsStore(db *gorm.DB) *PullRequestsStore {
	return &PullRequestsStore{db: db}
}

// Create creates the pull request, its issue with given labels and
// attachments, and its patch, tests whether the patch applies, notifies the
// watchers of the repository and the users it mentions, and prepares
// webhooks.
func (s *PullRequestsStore) Create(ctx context.Context, logger *logx.Logger, repo *Repository, pull *Issue, labelIDs []int64, uuids []string, pr *PullRequest, patch []byte) error {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := newIssuesStore(tx).create(ctx, NewIssueOptions{
			Repo:        repo,
			Issue:       pull,
			LableIDs:    labelIDs,
			Attachments: uuids,
			IsPull:      true,
		})
		if err != nil {
			return errors.Wrap(err, "create issue")
		}

		pr.Index = pull.Index
		if err = repo.SavePatch(pr.Index, patch); err != nil {
			return errors.Wrap(err, "save patch")
		}

		pr.BaseRepo = repo
		if err = newPullRequestsStore(tx).testPatch(ctx, logger, pr); err != nil {
			return errors.Wrap(err, "test patch")
		}
		// No conflict appears after test means mergeable.
		if pr.Status == PullRequestStatusChecking {
			pr.Status = PullRequestStatusMergeable
		}

		pr.IssueID = pull.ID
		return tx.Create(pr).Error
	})
	if err != nil {
		return err
	}

	err = notifyWatchers(s.db.WithContext(ctx), &Action{
		ActUserID:    pull.Poster.ID,
		ActUserName:  pull.Poster.Name,
		OpType:       ActionCreatePullRequest,
		Content:      fmt.Sprintf("%d|%s", pull.Index, pull.Title),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	})
	if err != nil {
		logger.Error("Failed to notify watchers", "error", err)
	}
	if err = newIssuesStore(s.db).mailParticipants(ctx, logger, pull); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}

	pr.Issue = pull
	pull.PullRequest = pr
	err = newWebhooksStore(s.db).Prepare(ctx, logger, repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
		Action:      apiv1types.WebhookIssueOpened,
		Index:       pull.Index,
		PullRequest: pr.APIFormat(logger),
		Repository:  repo.APIFormatLegacy(logger, nil),
		Sender:      pull.Poster.APIFormat(),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
	}
	return nil
}

func (s *PullRequestsStore) get(ctx context.Context, args map[string]any, query string, values ...any) (*PullRequest, error) {
	pr := new(PullRequest)
	err := s.db.WithContext(ctx).Where(query, values...).First(pr).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPullRequestNotExist{args: args}
		}
		return nil, err
	}
	return pr, s.LoadAttributes(ctx, pr)
}

// GetByID returns the pull request with given ID and its attributes. It
// returns ErrPullRequestNotExist when not found.
func (s *PullRequestsStore) GetByID(ctx context.Context, id int64) (*PullRequest, error) {
	return s.get(ctx, map[string]any{"pullRequestID": id}, "id = ?", id)
}

// GetByIssueID returns the pull request of the issue with given ID and its
// attributes. It returns ErrPullRequestNotExist when not found.
func (s *PullRequestsStore) GetByIssueID(ctx context.Context, issueID int64) (*PullRequest, error) {
	return s.get(ctx, map[string]any{"issueID": issueID}, "issue_id = ?", issueID)
}

// openUnmerged returns the query of pull requests that are open and not
// merged.
func (s *PullRequestsStore) openUnmerged(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Select("pull_request.*").
		Joins("INNER JOIN issue ON issue.id = pull_request.issue_id").
		Where("pull_request.has_merged = ? AND issue.is_closed = ?", false, false)
}

// GetUnmerged returns the pull request that is open and not merged from given
// head branch to given base branch, without its attributes. It returns
// ErrPullRequestNotExist when not found.
func (s *PullRequestsStore) GetUnmerged(ctx context.Context, headRepoID, baseRepoID int64, headBranch, baseBranch string) (*PullRequest, error) {
	pr := new(PullRequest)
	err := s.openUnmerged(ctx).
		Where("pull_request.head_repo_id = ? AND pull_request.head_branch = ?", headRepoID, headBranch).
		Where("pull_request.base_repo_id = ? AND pull_request.base_branch = ?", baseRepoID, baseBranch).
		First(pr).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPullRequestNotExist{args: map[string]any{
				"headRepoID": headRepoID,
				"baseRepoID": baseRepoID,
				"headBranch": headBranch,
				"baseBranch": baseBranch,
			}}
		}
		return nil, err
	}
	return pr, nil
}

// listUnmergedByHead returns the pull requests that are open and not merged
// from given branch of the repository with given ID, without their
// attributes.
func (s *PullRequestsStore) listUnmergedByHead(ctx context.Context, repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, s.openUnmerged(ctx).
		Where("pull_request.head_repo_id = ? AND pull_request.head_branch = ?", repoID, branch).
		Order("pull_request.id ASC").
		Find(&prs).Error
}

// listUnmergedByBase returns the pull requests that are open and not merged
// into given branch of the repository with given ID, without their
// attributes.
func (s *PullRequestsStore) listUnmergedByBase(ctx context.Context, repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, s.openUnmerged(ctx).
		Where("pull_request.base_repo_id = ? AND pull_request.base_branch = ?", repoID, branch).
		Order("pull_request.id ASC").
		Find(&prs).Error
}

// LoadAttributes sets the head and base repositories with their attributes,
// and the merger of merged ones, of given pull requests that are not set yet.
// A head repository that no longer exists is left unset, and a merger that no
// longer exists is the ghost user. Their issues are not loaded.
func (s *PullRequestsStore) LoadAttributes(ctx context.Context, prs ...*PullRequest) (err error) {
	db := s.db.WithContext(ctx)
	for _, pr := range prs {
		if pr.HeadRepo == nil {
			pr.HeadRepo, err = getRepositoryByID(db, pr.HeadRepoID)
			if err != nil && !IsErrRepoNotExist(err) {
				return errors.Wrapf(err, "get head repository %d", pr.HeadRepoID)
			}
		}

		if pr.BaseRepo == nil {
			pr.BaseRepo, err = getRepositoryByID(db, pr.BaseRepoID)
			if err != nil {
				return errors.Wrapf(err, "get base repository %d", pr.BaseRepoID)
			}
		}

		if pr.HasMerged && pr.Merger == nil {
			pr.Merger, err = newUsersStore(s.db).GetByID(ctx, pr.MergerID)
			if err != nil {
				if !IsErrUserNotExist(err) {
					return errors.Wrapf(err, "get merger %d", pr.MergerID)
				}
				pr.MergerID = -1
				pr.Merger = NewGhostUser()
			}
		}
	}
	return nil
}

// Update saves all fields of the pull request.
func (s *PullRequestsStore) Update(ctx context.Context, pr *PullRequest) error {
	return s.db.WithContext(ctx).Model(pr).Select("*").Updates(pr).Error
}

// updateStatus saves the status of the pull request.
func (s *PullRequestsStore) updateStatus(ctx context.Context, pr *PullRequest) error {
	return s.db.WithContext(ctx).Model(pr).Update("status", pr.Status).Error
}

// UpdatePatch saves the merge base and the patch of the pull request
// between its base and head branches.
func (s *PullRequestsStore) UpdatePatch(ctx context.Context, logger *logx.Logger, pr *PullRequest) (err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	headGitRepo, err := git.Open(pr.HeadRepo.RepoPath())
	if err != nil {
		return errors.Newf("open repository: %v", err)
	}

	// Add a temporary remote.
	tmpRemote := strconv.FormatInt(time.Now().UnixNano(), 10)
	baseRepoPath := RepoPath(pr.BaseRepo.MustOwner().Name, pr.BaseRepo.Name)
	err = headGitRepo.RemoteAdd(ctx, tmpRemote, baseRepoPath, git.RemoteAddOptions{Fetch: true})
	if err != nil {
		return errors.Newf("add remote %q [repo_id: %d]: %v", tmpRemote, pr.HeadRepoID, err)
	}
	defer func() {
		if err := headGitRepo.RemoteRemove(ctx, tmpRemote); err != nil {
			logger.Error("Failed to remove temporary remote", "remote", tmpRemote, "repoID", pr.HeadRepoID, "error", err)
		}
	}()

	remoteBranch := "remotes/" + tmpRemote + "/" + pr.BaseBranch
	pr.MergeBase, err = headGitRepo.MergeBase(ctx, remoteBranch, pr.HeadBranch)
	if err != nil {
		return errors.Newf("get merge base: %v", err)
	}
	// Save only the merge base, so a merge that finished meanwhile is not
	// overwritten.
	err = s.db.WithContext(ctx).Model(pr).UpdateColumn("merge_base", pr.MergeBase).Error
	if err != nil {
		return errors.Wrap(err, "update pull request")
	}

	patch, err := headGitRepo.DiffBinary(ctx, pr.MergeBase, pr.HeadBranch)
	if err != nil {
		return errors.Newf("get binary patch: %v", err)
	}

	if err = pr.BaseRepo.SavePatch(pr.Index, patch); err != nil {
		return errors.Newf("save patch: %v", err)
	}

	logger.Debug("Patch saved", "pullRequestID", pr.ID)
	return nil
}

// AddToTaskQueue marks the pull request as being checked and queues testing
// its patch.
func (s *PullRequestsStore) AddToTaskQueue(ctx context.Context, logger *logx.Logger, pr *PullRequest) {
	ctx = context.WithoutCancel(ctx)
	go PullRequestQueue.AddFunc(pr.ID, func() {
		pr.Status = PullRequestStatusChecking
		if err := s.updateStatus(ctx, pr); err != nil {
			logger.Error("Failed to update pull request status", "pullRequestID", pr.ID, "error", err)
		}
	})
}

// addHeadRepoTasks updates the patches of given pull requests, pushes their
// head branches to their base repositories and queues testing them. Pull
// requests whose head repository is not loaded are skipped.
func (s *PullRequestsStore) addHeadRepoTasks(ctx context.Context, logger *logx.Logger, prs []*PullRequest) {
	for _, pr := range prs {
		if pr.HeadRepo == nil {
			logger.Debug("Skipped pull request with missing head repository", "pullRequestID", pr.ID)
			continue
		}

		logger.Debug("Composing new test task", "pullRequestID", pr.ID)
		if err := s.UpdatePatch(ctx, logger, pr); err != nil {
			logger.Error("Failed to update patch", "error", err)
			continue
		} else if err := pr.PushToBaseRepo(ctx, logger); err != nil {
			logger.Error("Failed to push to base repository", "error", err)
			continue
		}

		s.AddToTaskQueue(ctx, logger, pr)
	}
}

// checkAndUpdateStatus saves the status of the tested pull request, mergeable
// if the test found no conflict, unless it is queued for another test.
func (s *PullRequestsStore) checkAndUpdateStatus(ctx context.Context, logger *logx.Logger, pr *PullRequest) {
	if pr.Status == PullRequestStatusChecking {
		pr.Status = PullRequestStatusMergeable
	}

	if !PullRequestQueue.Exist(pr.ID) {
		if err := s.updateStatus(ctx, pr); err != nil {
			logger.Error("Failed to update pull request status", "pullRequestID", pr.ID, "error", err)
		}
	}
}
