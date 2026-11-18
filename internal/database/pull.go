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
	"unknwon.dev/x/logx"
	"xorm.io/xorm"

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

func (pr *PullRequest) BeforeUpdate() {
	pr.MergedUnix = pr.Merged.Unix()
}

// Note: don't try to get Issue because it will end up in recursive querying.
func (pr *PullRequest) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "merged_unix":
		if !pr.HasMerged {
			return
		}

		pr.Merged = time.Unix(pr.MergedUnix, 0).Local()
	}
}

// Note: don't try to get Issue because it will end up in recursive querying.
func (pr *PullRequest) loadAttributes(e Engine) (err error) {
	if pr.HeadRepo == nil {
		pr.HeadRepo, err = getRepositoryByID(legacyTx(e), pr.HeadRepoID)
		if err != nil && !IsErrRepoNotExist(err) {
			return errors.Newf("get head repository by ID: %v", err)
		}
	}

	if pr.BaseRepo == nil {
		pr.BaseRepo, err = getRepositoryByID(legacyTx(e), pr.BaseRepoID)
		if err != nil {
			return errors.Newf("get base repository by ID: %v", err)
		}
	}

	if pr.HasMerged && pr.Merger == nil {
		pr.Merger, err = newUsersStore(legacyTx(e)).GetByID(context.TODO(), pr.MergerID)
		if IsErrUserNotExist(err) {
			pr.MergerID = -1
			pr.Merger = NewGhostUser()
		} else if err != nil {
			return errors.Newf("get merger by ID: %v", err)
		}
	}

	return nil
}

func (pr *PullRequest) LoadAttributes() error {
	return pr.loadAttributes(x)
}

func (pr *PullRequest) LoadIssue() (err error) {
	if pr.Issue != nil {
		return nil
	}

	pr.Issue, err = GetIssueByID(pr.IssueID)
	return err
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

// Merge merges pull request to base repository. The message is used for the
// commit that merge commits and squashing create, and its default is used
// when its title is empty.
//
// FIXME: use repoWorkingPool to make sure two merges do not happen at same time.
func (pr *PullRequest) Merge(ctx context.Context, logger *logx.Logger, doer *User, baseGitRepo *git.Repository, mergeStyle MergeStyle, message MergeMessage) (err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	defer func() {
		go HookQueue.Add(pr.BaseRepo.ID)
		go AddTestPullRequestTask(logger, doer, pr.BaseRepo.ID, pr.BaseBranch, false)
	}()

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = pr.Issue.changeStatus(logger, sess, doer, pr.Issue.Repo, true); err != nil {
		return errors.Newf("Issue.changeStatus: %v", err)
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
	if _, err = sess.ID(pr.ID).AllCols().Update(pr); err != nil {
		return errors.Newf("update pull request: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return errors.Newf("commit: %v", err)
	}

	if err = Handle.Actions().MergePullRequest(ctx, doer, pr.Issue.Repo.Owner, pr.Issue.Repo, pr.Issue); err != nil {
		logger.Error("Failed to create action for merging pull request", "pullRequestID", pr.ID, "error", err)
	}

	// Reload pull request information.
	if err = pr.LoadAttributes(); err != nil {
		logger.Error("Failed to load pull request attributes", "pullRequestID", pr.ID, "error", err)
		return nil
	}
	if err = Handle.Webhooks().Prepare(ctx, logger, pr.Issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
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

	pcs, err := CommitsToPushCommits(commits).APIFormat(ctx, Handle.Users(), pr.BaseRepo.RepoPath(), pr.BaseRepo.HTMLURL())
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
	if err = Handle.Webhooks().Prepare(ctx, logger, pr.BaseRepo, HookEventTypePush, p); err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
		return nil
	}
	return nil
}

// testPatch checks if patch can be merged to base repository without conflict.
func (pr *PullRequest) testPatch(ctx context.Context, logger *logx.Logger) (err error) {
	if pr.BaseRepo == nil {
		pr.BaseRepo, err = getRepositoryByID(Handle.db, pr.BaseRepoID)
		if err != nil {
			return errors.Newf("GetRepositoryByID: %v", err)
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

// NewPullRequest creates new pull request with labels for repository.
func NewPullRequest(ctx context.Context, logger *logx.Logger, repo *Repository, pull *Issue, labelIDs []int64, uuids []string, pr *PullRequest, patch []byte) (err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssue(sess, NewIssueOptions{
		Repo:        repo,
		Issue:       pull,
		LableIDs:    labelIDs,
		Attachments: uuids,
		IsPull:      true,
	}); err != nil {
		return errors.Newf("newIssue: %v", err)
	}

	pr.Index = pull.Index
	if err = repo.SavePatch(pr.Index, patch); err != nil {
		return errors.Newf("SavePatch: %v", err)
	}

	pr.BaseRepo = repo
	if err = pr.testPatch(ctx, logger); err != nil {
		return errors.Newf("testPatch: %v", err)
	}
	// No conflict appears after test means mergeable.
	if pr.Status == PullRequestStatusChecking {
		pr.Status = PullRequestStatusMergeable
	}

	pr.IssueID = pull.ID
	if _, err = sess.Insert(pr); err != nil {
		return errors.Newf("insert pull repo: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return errors.Newf("commit: %v", err)
	}

	if err = NotifyWatchers(&Action{
		ActUserID:    pull.Poster.ID,
		ActUserName:  pull.Poster.Name,
		OpType:       ActionCreatePullRequest,
		Content:      fmt.Sprintf("%d|%s", pull.Index, pull.Title),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	}); err != nil {
		logger.Error("Failed to notify watchers", "error", err)
	}
	if err = pull.MailParticipants(logger); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}

	pr.Issue = pull
	pull.PullRequest = pr
	if err = Handle.Webhooks().Prepare(ctx, logger, repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
		Action:      apiv1types.WebhookIssueOpened,
		Index:       pull.Index,
		PullRequest: pr.APIFormat(logger),
		Repository:  repo.APIFormatLegacy(logger, nil),
		Sender:      pull.Poster.APIFormat(),
	}); err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
	}

	return nil
}

// GetUnmergedPullRequest returns a pull request that is open and has not been merged
// by given head/base and repo/branch.
func GetUnmergedPullRequest(headRepoID, baseRepoID int64, headBranch, baseBranch string) (*PullRequest, error) {
	pr := new(PullRequest)
	has, err := x.Where("head_repo_id=? AND head_branch=? AND base_repo_id=? AND base_branch=? AND has_merged=? AND issue.is_closed=?",
		headRepoID, headBranch, baseRepoID, baseBranch, false, false).
		Join("INNER", "issue", "issue.id=pull_request.issue_id").Get(pr)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrPullRequestNotExist{args: map[string]any{
			"headRepoID": headRepoID,
			"baseRepoID": baseRepoID,
			"headBranch": headBranch,
			"baseBranch": baseBranch,
		}}
	}

	return pr, nil
}

// GetUnmergedPullRequestsByHeadInfo returns all pull requests that are open and have not been merged
// by given head information (repo and branch).
func GetUnmergedPullRequestsByHeadInfo(repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, x.Where("head_repo_id = ? AND head_branch = ? AND has_merged = ? AND issue.is_closed = ?",
		repoID, branch, false, false).
		Join("INNER", "issue", "issue.id = pull_request.issue_id").Find(&prs)
}

// GetUnmergedPullRequestsByBaseInfo returns all pull requests that are open and have not been merged
// by given base information (repo and branch).
func GetUnmergedPullRequestsByBaseInfo(repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, x.Where("base_repo_id=? AND base_branch=? AND has_merged=? AND issue.is_closed=?",
		repoID, branch, false, false).
		Join("INNER", "issue", "issue.id=pull_request.issue_id").Find(&prs)
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

func getPullRequestByID(e Engine, id int64) (*PullRequest, error) {
	pr := new(PullRequest)
	has, err := e.ID(id).Get(pr)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrPullRequestNotExist{args: map[string]any{"pullRequestID": id}}
	}
	return pr, pr.loadAttributes(e)
}

// GetPullRequestByID returns a pull request by given ID.
func GetPullRequestByID(id int64) (*PullRequest, error) {
	return getPullRequestByID(x, id)
}

func getPullRequestByIssueID(e Engine, issueID int64) (*PullRequest, error) {
	pr := &PullRequest{
		IssueID: issueID,
	}
	has, err := e.Get(pr)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrPullRequestNotExist{args: map[string]any{"issueID": issueID}}
	}
	return pr, pr.loadAttributes(e)
}

// GetPullRequestByIssueID returns pull request by given issue ID.
func GetPullRequestByIssueID(issueID int64) (*PullRequest, error) {
	return getPullRequestByIssueID(x, issueID)
}

// Update updates all fields of pull request.
func (pr *PullRequest) Update() error {
	_, err := x.Id(pr.ID).AllCols().Update(pr)
	return err
}

// UpdateCols updates specific fields of pull request.
func (pr *PullRequest) UpdateCols(cols ...string) error {
	_, err := x.Id(pr.ID).Cols(cols...).Update(pr)
	return err
}

// UpdatePatch generates and saves a new patch.
func (pr *PullRequest) UpdatePatch(ctx context.Context, logger *logx.Logger) (err error) {
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
	} else if err = pr.Update(); err != nil {
		return errors.Newf("update: %v", err)
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

// AddToTaskQueue adds itself to pull request test task queue.
func (pr *PullRequest) AddToTaskQueue(logger *logx.Logger) {
	go PullRequestQueue.AddFunc(pr.ID, func() {
		pr.Status = PullRequestStatusChecking
		if err := pr.UpdateCols("status"); err != nil {
			logger.Error("Failed to update pull request status", "pullRequestID", pr.ID, "error", err)
		}
	})
}

type PullRequestList []*PullRequest

func (prs PullRequestList) loadAttributes(e Engine) (err error) {
	if len(prs) == 0 {
		return nil
	}

	// Load issues.
	set := make(map[int64]*Issue)
	for i := range prs {
		set[prs[i].IssueID] = nil
	}
	issueIDs := make([]int64, 0, len(prs))
	for issueID := range set {
		issueIDs = append(issueIDs, issueID)
	}
	issues := make([]*Issue, 0, len(issueIDs))
	if err = e.Where("id > 0").In("id", issueIDs).Find(&issues); err != nil {
		return errors.Newf("find issues: %v", err)
	}
	for i := range issues {
		set[issues[i].ID] = issues[i]
	}
	for i := range prs {
		prs[i].Issue = set[prs[i].IssueID]
	}

	for i := range prs {
		if err = prs[i].loadAttributes(e); err != nil {
			return errors.Newf("loadAttributes [%d]: %v", prs[i].ID, err)
		}
	}

	return nil
}

func (prs PullRequestList) LoadAttributes() error {
	return prs.loadAttributes(x)
}

func addHeadRepoTasks(logger *logx.Logger, prs []*PullRequest) {
	for _, pr := range prs {
		if pr.HeadRepo == nil {
			logger.Debug("Skipped pull request with missing head repository", "pullRequestID", pr.ID)
			continue
		}

		logger.Debug("Composing new test task", "pullRequestID", pr.ID)
		if err := pr.UpdatePatch(context.Background(), logger); err != nil {
			logger.Error("Failed to update patch", "error", err)
			continue
		} else if err := pr.PushToBaseRepo(context.Background(), logger); err != nil {
			logger.Error("Failed to push to base repository", "error", err)
			continue
		}

		pr.AddToTaskQueue(logger)
	}
}

// AddTestPullRequestTask adds new test tasks for pull requests whose head or
// base is the given repository and branch, and generates new patches for
// testing as needed.
func AddTestPullRequestTask(logger *logx.Logger, doer *User, repoID int64, branch string, isSync bool) {
	logger.Debug("Finding pull requests by head branch", "headRepoID", repoID, "headBranch", branch)
	prs, err := GetUnmergedPullRequestsByHeadInfo(repoID, branch)
	if err != nil {
		logger.Error("Failed to find pull requests", "headRepoID", repoID, "headBranch", branch, "error", err)
		return
	}

	if isSync {
		if err = PullRequestList(prs).LoadAttributes(); err != nil {
			logger.Error("Failed to load pull request attributes", "error", err)
		}

		if err == nil {
			for _, pr := range prs {
				pr.Issue.PullRequest = pr
				if err = pr.Issue.LoadAttributes(); err != nil {
					logger.Error("Failed to load issue attributes", "error", err)
					continue
				}
				if err = Handle.Webhooks().Prepare(context.TODO(), logger, pr.Issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
					Action:      apiv1types.WebhookIssueSynchronized,
					Index:       pr.Issue.Index,
					PullRequest: pr.Issue.PullRequest.APIFormat(logger),
					Repository:  pr.Issue.Repo.APIFormatLegacy(logger, nil),
					Sender:      doer.APIFormat(),
				}); err != nil {
					logger.Error("Failed to prepare webhooks", "pullRequestID", pr.ID, "error", err)
					continue
				}
			}
		}
	}

	addHeadRepoTasks(logger, prs)

	logger.Debug("Finding pull requests by base branch", "baseRepoID", repoID, "baseBranch", branch)
	prs, err = GetUnmergedPullRequestsByBaseInfo(repoID, branch)
	if err != nil {
		logger.Error("Failed to find pull requests", "baseRepoID", repoID, "baseBranch", branch, "error", err)
		return
	}
	for _, pr := range prs {
		pr.AddToTaskQueue(logger)
	}
}

// checkAndUpdateStatus checks if pull request is possible to leave checking
// status, and sets it to be either conflict or mergeable.
func (pr *PullRequest) checkAndUpdateStatus(logger *logx.Logger) {
	// Status is not changed to conflict means mergeable.
	if pr.Status == PullRequestStatusChecking {
		pr.Status = PullRequestStatusMergeable
	}

	// Make sure there is no waiting test to process before leaving the checking status.
	if !PullRequestQueue.Exist(pr.ID) {
		if err := pr.UpdateCols("status"); err != nil {
			logger.Error("Failed to update pull request status", "pullRequestID", pr.ID, "error", err)
		}
	}
}

// TestPullRequests checks and tests untested patches of pull requests.
// TODO: test more pull requests at same time.
func TestPullRequests(logger *logx.Logger) {
	prs := make([]*PullRequest, 0, 10)
	_ = x.Iterate(PullRequest{
		Status: PullRequestStatusChecking,
	},
		func(idx int, bean any) error {
			pr := bean.(*PullRequest)

			if err := pr.LoadAttributes(); err != nil {
				logger.Error("Failed to load pull request attributes", "error", err)
				return nil
			}

			if err := pr.testPatch(context.Background(), logger); err != nil {
				logger.Error("Failed to test patch", "error", err)
				return nil
			}
			prs = append(prs, pr)
			return nil
		})

	// Update pull request status.
	for _, pr := range prs {
		pr.checkAndUpdateStatus(logger)
	}

	// Start listening on new test requests.
	for prID := range PullRequestQueue.Queue() {
		logger.Debug("Processing test task", "pullRequestID", prID)
		PullRequestQueue.Remove(prID)

		id, _ := strconv.ParseInt(prID, 10, 64)
		pr, err := GetPullRequestByID(id)
		if err != nil {
			logger.Error("Failed to get pull request", "pullRequestID", prID, "error", err)
			continue
		} else if err = pr.testPatch(context.Background(), logger); err != nil {
			logger.Error("Failed to test patch", "pullRequestID", pr.ID, "error", err)
			continue
		}

		pr.checkAndUpdateStatus(logger)
	}
}

func InitTestPullRequests(logger *logx.Logger) {
	go TestPullRequests(logger)
}
