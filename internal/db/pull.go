// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"
	"xorm.io/xorm"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/sync"
)

var PullRequestQueue = sync.NewUniqueQueue(1000)

type PullRequestType int

const (
	PULL_REQUEST_GOGS PullRequestType = iota
	PLLL_ERQUEST_GIT
)

type PullRequestStatus int

const (
	PULL_REQUEST_STATUS_CONFLICT PullRequestStatus = iota
	PULL_REQUEST_STATUS_CHECKING
	PULL_REQUEST_STATUS_MERGEABLE
)

// PullRequest represents relation between pull request and repositories.
type PullRequest struct {
	ID     int64
	Type   PullRequestType
	Status PullRequestStatus

	IssueID int64  `xorm:"INDEX"`
	Issue   *Issue `xorm:"-" json:"-"`
	Index   int64

	HeadRepoID   int64
	HeadRepo     *Repository `xorm:"-" json:"-"`
	BaseRepoID   int64
	BaseRepo     *Repository `xorm:"-" json:"-"`
	HeadUserName string
	HeadBranch   string
	BaseBranch   string
	MergeBase    string `xorm:"VARCHAR(40)"`

	HasMerged      bool
	MergedCommitID string `xorm:"VARCHAR(40)"`
	MergerID       int64
	Merger         *User     `xorm:"-" json:"-"`
	Merged         time.Time `xorm:"-" json:"-"`
	MergedUnix     int64
}

func (pr *PullRequest) BeforeUpdate() {
	pr.MergedUnix = pr.Merged.Unix()
}

// Note: don't try to get Issue because will end up recursive querying.
func (pr *PullRequest) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "merged_unix":
		if !pr.HasMerged {
			return
		}

		pr.Merged = time.Unix(pr.MergedUnix, 0).Local()
	}
}

// Note: don't try to get Issue because will end up recursive querying.
func (pr *PullRequest) loadAttributes(e Engine) (err error) {
	if pr.HeadRepo == nil {
		pr.HeadRepo, err = getRepositoryByID(e, pr.HeadRepoID)
		if err != nil && !IsErrRepoNotExist(err) {
			return fmt.Errorf("get head repository by ID: %v", err)
		}
	}

	if pr.BaseRepo == nil {
		pr.BaseRepo, err = getRepositoryByID(e, pr.BaseRepoID)
		if err != nil {
			return fmt.Errorf("get base repository by ID: %v", err)
		}
	}

	if pr.HasMerged && pr.Merger == nil {
		pr.Merger, err = getUserByID(e, pr.MergerID)
		if IsErrUserNotExist(err) {
			pr.MergerID = -1
			pr.Merger = NewGhostUser()
		} else if err != nil {
			return fmt.Errorf("get merger by ID: %v", err)
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

// This method assumes following fields have been assigned with valid values:
// Required - Issue, BaseRepo
// Optional - HeadRepo, Merger
func (pr *PullRequest) APIFormat() *api.PullRequest {
	// In case of head repo has been deleted.
	var apiHeadRepo *api.Repository
	if pr.HeadRepo == nil {
		apiHeadRepo = &api.Repository{
			Name: "deleted",
		}
	} else {
		apiHeadRepo = pr.HeadRepo.APIFormat(nil)
	}

	apiIssue := pr.Issue.APIFormat()
	apiPullRequest := &api.PullRequest{
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
		BaseRepo:   pr.BaseRepo.APIFormat(nil),
		HTMLURL:    pr.Issue.HTMLURL(),
		HasMerged:  pr.HasMerged,
	}

	if pr.Status != PULL_REQUEST_STATUS_CHECKING {
		mergeable := pr.Status != PULL_REQUEST_STATUS_CONFLICT
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
	return pr.Status == PULL_REQUEST_STATUS_CHECKING
}

// CanAutoMerge returns true if this pull request can be merged automatically.
func (pr *PullRequest) CanAutoMerge() bool {
	return pr.Status == PULL_REQUEST_STATUS_MERGEABLE
}

// MergeStyle represents the approach to merge commits into base branch.
type MergeStyle string

const (
	MERGE_STYLE_REGULAR MergeStyle = "create_merge_commit"
	MERGE_STYLE_REBASE  MergeStyle = "rebase_before_merging"
)

// Merge merges pull request to base repository.
// FIXME: add repoWorkingPull make sure two merges does not happen at same time.
func (pr *PullRequest) Merge(doer *User, baseGitRepo *git.Repository, mergeStyle MergeStyle, commitDescription string) (err error) {
	defer func() {
		go HookQueue.Add(pr.BaseRepo.ID)
		go AddTestPullRequestTask(doer, pr.BaseRepo.ID, pr.BaseBranch, false)
	}()

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = pr.Issue.changeStatus(sess, doer, pr.Issue.Repo, true); err != nil {
		return fmt.Errorf("Issue.changeStatus: %v", err)
	}

	headRepoPath := RepoPath(pr.HeadUserName, pr.HeadRepo.Name)
	headGitRepo, err := git.Open(headRepoPath)
	if err != nil {
		return fmt.Errorf("open repository: %v", err)
	}

	// Create temporary directory to store temporary copy of the base repository,
	// and clean it up when operation finished regardless of succeed or not.
	tmpBasePath := filepath.Join(conf.Server.AppDataPath, "tmp", "repos", com.ToStr(time.Now().Nanosecond())+".git")
	if err = os.MkdirAll(filepath.Dir(tmpBasePath), os.ModePerm); err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(filepath.Dir(tmpBasePath))
	}()

	// Clone the base repository to the defined temporary directory,
	// and checks out to base branch directly.
	var stderr string
	if _, stderr, err = process.ExecTimeout(5*time.Minute,
		fmt.Sprintf("PullRequest.Merge (git clone): %s", tmpBasePath),
		"git", "clone", "-b", pr.BaseBranch, baseGitRepo.Path(), tmpBasePath); err != nil {
		return fmt.Errorf("git clone: %s", stderr)
	}

	// Add remote which points to the head repository.
	if _, stderr, err = process.ExecDir(-1, tmpBasePath,
		fmt.Sprintf("PullRequest.Merge (git remote add): %s", tmpBasePath),
		"git", "remote", "add", "head_repo", headRepoPath); err != nil {
		return fmt.Errorf("git remote add [%s -> %s]: %s", headRepoPath, tmpBasePath, stderr)
	}

	// Fetch information from head repository to the temporary copy.
	if _, stderr, err = process.ExecDir(-1, tmpBasePath,
		fmt.Sprintf("PullRequest.Merge (git fetch): %s", tmpBasePath),
		"git", "fetch", "head_repo"); err != nil {
		return fmt.Errorf("git fetch [%s -> %s]: %s", headRepoPath, tmpBasePath, stderr)
	}

	remoteHeadBranch := "head_repo/" + pr.HeadBranch

	// Check if merge style is allowed, reset to default style if not
	if mergeStyle == MERGE_STYLE_REBASE && !pr.BaseRepo.PullsAllowRebase {
		mergeStyle = MERGE_STYLE_REGULAR
	}

	switch mergeStyle {
	case MERGE_STYLE_REGULAR: // Create merge commit

		// Merge changes from head branch.
		if _, stderr, err = process.ExecDir(-1, tmpBasePath,
			fmt.Sprintf("PullRequest.Merge (git merge --no-ff --no-commit): %s", tmpBasePath),
			"git", "merge", "--no-ff", "--no-commit", remoteHeadBranch); err != nil {
			return fmt.Errorf("git merge --no-ff --no-commit [%s]: %v - %s", tmpBasePath, err, stderr)
		}

		// Create a merge commit for the base branch.
		sig := doer.NewGitSig()
		if _, stderr, err = process.ExecDir(-1, tmpBasePath,
			fmt.Sprintf("PullRequest.Merge (git merge): %s", tmpBasePath),
			"git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
			"-m", fmt.Sprintf("Merge branch '%s' of %s/%s into %s", pr.HeadBranch, pr.HeadUserName, pr.HeadRepo.Name, pr.BaseBranch),
			"-m", commitDescription); err != nil {
			return fmt.Errorf("git commit [%s]: %v - %s", tmpBasePath, err, stderr)
		}

	case MERGE_STYLE_REBASE: // Rebase before merging

		// Rebase head branch based on base branch, this creates a non-branch commit state.
		if _, stderr, err = process.ExecDir(-1, tmpBasePath,
			fmt.Sprintf("PullRequest.Merge (git rebase): %s", tmpBasePath),
			"git", "rebase", "--quiet", pr.BaseBranch, remoteHeadBranch); err != nil {
			return fmt.Errorf("git rebase [%s on %s]: %s", remoteHeadBranch, pr.BaseBranch, stderr)
		}

		// Name non-branch commit state to a new temporary branch in order to save changes.
		tmpBranch := com.ToStr(time.Now().UnixNano(), 10)
		if _, stderr, err = process.ExecDir(-1, tmpBasePath,
			fmt.Sprintf("PullRequest.Merge (git checkout): %s", tmpBasePath),
			"git", "checkout", "-b", tmpBranch); err != nil {
			return fmt.Errorf("git checkout '%s': %s", tmpBranch, stderr)
		}

		// Check out the base branch to be operated on.
		if _, stderr, err = process.ExecDir(-1, tmpBasePath,
			fmt.Sprintf("PullRequest.Merge (git checkout): %s", tmpBasePath),
			"git", "checkout", pr.BaseBranch); err != nil {
			return fmt.Errorf("git checkout '%s': %s", pr.BaseBranch, stderr)
		}

		// Merge changes from temporary branch to the base branch.
		if _, stderr, err = process.ExecDir(-1, tmpBasePath,
			fmt.Sprintf("PullRequest.Merge (git merge): %s", tmpBasePath),
			"git", "merge", tmpBranch); err != nil {
			return fmt.Errorf("git merge [%s]: %v - %s", tmpBasePath, err, stderr)
		}

	default:
		return fmt.Errorf("unknown merge style: %s", mergeStyle)
	}

	// Push changes on base branch to upstream.
	if _, stderr, err = process.ExecDir(-1, tmpBasePath,
		fmt.Sprintf("PullRequest.Merge (git push): %s", tmpBasePath),
		"git", "push", baseGitRepo.Path(), pr.BaseBranch); err != nil {
		return fmt.Errorf("git push: %s", stderr)
	}

	pr.MergedCommitID, err = headGitRepo.BranchCommitID(pr.HeadBranch)
	if err != nil {
		return fmt.Errorf("get head branch %q commit ID: %v", pr.HeadBranch, err)
	}

	pr.HasMerged = true
	pr.Merged = time.Now()
	pr.MergerID = doer.ID
	if _, err = sess.ID(pr.ID).AllCols().Update(pr); err != nil {
		return fmt.Errorf("update pull request: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	if err = MergePullRequestAction(doer, pr.Issue.Repo, pr.Issue); err != nil {
		log.Error("MergePullRequestAction [%d]: %v", pr.ID, err)
	}

	// Reload pull request information.
	if err = pr.LoadAttributes(); err != nil {
		log.Error("LoadAttributes: %v", err)
		return nil
	}
	if err = PrepareWebhooks(pr.Issue.Repo, HOOK_EVENT_PULL_REQUEST, &api.PullRequestPayload{
		Action:      api.HOOK_ISSUE_CLOSED,
		Index:       pr.Index,
		PullRequest: pr.APIFormat(),
		Repository:  pr.Issue.Repo.APIFormat(nil),
		Sender:      doer.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks: %v", err)
		return nil
	}

	commits, err := headGitRepo.RevList([]string{pr.MergeBase + "..." + pr.MergedCommitID})
	if err != nil {
		log.Error("Failed to list commits [merge_base: %s, merged_commit_id: %s]: %v", pr.MergeBase, pr.MergedCommitID, err)
		return nil
	}

	// NOTE: It is possible that head branch is not fully sync with base branch
	// for merge commits, so we need to get latest head commit and append merge
	// commit manually to avoid strange diff commits produced.
	mergeCommit, err := baseGitRepo.BranchCommit(pr.BaseBranch)
	if err != nil {
		log.Error("Failed to get base branch %q commit: %v", pr.BaseBranch, err)
		return nil
	}
	if mergeStyle == MERGE_STYLE_REGULAR {
		commits = append([]*git.Commit{mergeCommit}, commits...)
	}

	pcs, err := CommitsToPushCommits(commits).ToApiPayloadCommits(pr.BaseRepo.RepoPath(), pr.BaseRepo.HTMLURL())
	if err != nil {
		log.Error("Failed to convert to API payload commits: %v", err)
		return nil
	}

	p := &api.PushPayload{
		Ref:        git.RefsHeads + pr.BaseBranch,
		Before:     pr.MergeBase,
		After:      mergeCommit.ID.String(),
		CompareURL: conf.Server.ExternalURL + pr.BaseRepo.ComposeCompareURL(pr.MergeBase, pr.MergedCommitID),
		Commits:    pcs,
		Repo:       pr.BaseRepo.APIFormat(nil),
		Pusher:     pr.HeadRepo.MustOwner().APIFormat(),
		Sender:     doer.APIFormat(),
	}
	if err = PrepareWebhooks(pr.BaseRepo, HOOK_EVENT_PUSH, p); err != nil {
		log.Error("Failed to prepare webhooks: %v", err)
		return nil
	}
	return nil
}

// testPatch checks if patch can be merged to base repository without conflict.
// FIXME: make a mechanism to clean up stable local copies.
func (pr *PullRequest) testPatch() (err error) {
	if pr.BaseRepo == nil {
		pr.BaseRepo, err = GetRepositoryByID(pr.BaseRepoID)
		if err != nil {
			return fmt.Errorf("GetRepositoryByID: %v", err)
		}
	}

	patchPath, err := pr.BaseRepo.PatchPath(pr.Index)
	if err != nil {
		return fmt.Errorf("BaseRepo.PatchPath: %v", err)
	}

	// Fast fail if patch does not exist, this assumes data is corrupted.
	if !osutil.IsFile(patchPath) {
		log.Trace("PullRequest[%d].testPatch: ignored corrupted data", pr.ID)
		return nil
	}

	repoWorkingPool.CheckIn(com.ToStr(pr.BaseRepoID))
	defer repoWorkingPool.CheckOut(com.ToStr(pr.BaseRepoID))

	log.Trace("PullRequest[%d].testPatch (patchPath): %s", pr.ID, patchPath)

	if err := pr.BaseRepo.UpdateLocalCopyBranch(pr.BaseBranch); err != nil {
		return fmt.Errorf("UpdateLocalCopy [%d]: %v", pr.BaseRepoID, err)
	}

	args := []string{"apply", "--check"}
	if pr.BaseRepo.PullsIgnoreWhitespace {
		args = append(args, "--ignore-whitespace")
	}
	args = append(args, patchPath)

	pr.Status = PULL_REQUEST_STATUS_CHECKING
	_, stderr, err := process.ExecDir(-1, pr.BaseRepo.LocalCopyPath(),
		fmt.Sprintf("testPatch (git apply --check): %d", pr.BaseRepo.ID),
		"git", args...)
	if err != nil {
		log.Trace("PullRequest[%d].testPatch (apply): has conflict\n%s", pr.ID, stderr)
		pr.Status = PULL_REQUEST_STATUS_CONFLICT
		return nil
	}
	return nil
}

// NewPullRequest creates new pull request with labels for repository.
func NewPullRequest(repo *Repository, pull *Issue, labelIDs []int64, uuids []string, pr *PullRequest, patch []byte) (err error) {
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
		return fmt.Errorf("newIssue: %v", err)
	}

	pr.Index = pull.Index
	if err = repo.SavePatch(pr.Index, patch); err != nil {
		return fmt.Errorf("SavePatch: %v", err)
	}

	pr.BaseRepo = repo
	if err = pr.testPatch(); err != nil {
		return fmt.Errorf("testPatch: %v", err)
	}
	// No conflict appears after test means mergeable.
	if pr.Status == PULL_REQUEST_STATUS_CHECKING {
		pr.Status = PULL_REQUEST_STATUS_MERGEABLE
	}

	pr.IssueID = pull.ID
	if _, err = sess.Insert(pr); err != nil {
		return fmt.Errorf("insert pull repo: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	if err = NotifyWatchers(&Action{
		ActUserID:    pull.Poster.ID,
		ActUserName:  pull.Poster.Name,
		OpType:       ACTION_CREATE_PULL_REQUEST,
		Content:      fmt.Sprintf("%d|%s", pull.Index, pull.Title),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	}); err != nil {
		log.Error("NotifyWatchers: %v", err)
	}
	if err = pull.MailParticipants(); err != nil {
		log.Error("MailParticipants: %v", err)
	}

	pr.Issue = pull
	pull.PullRequest = pr
	if err = PrepareWebhooks(repo, HOOK_EVENT_PULL_REQUEST, &api.PullRequestPayload{
		Action:      api.HOOK_ISSUE_OPENED,
		Index:       pull.Index,
		PullRequest: pr.APIFormat(),
		Repository:  repo.APIFormat(nil),
		Sender:      pull.Poster.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks: %v", err)
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
		return nil, ErrPullRequestNotExist{args: map[string]interface{}{
			"headRepoID": headRepoID,
			"baseRepoID": baseRepoID,
			"headBranch": headBranch,
			"baseBranch": baseBranch,
		}}
	}

	return pr, nil
}

// GetUnmergedPullRequestsByHeadInfo returns all pull requests that are open and has not been merged
// by given head information (repo and branch).
func GetUnmergedPullRequestsByHeadInfo(repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, x.Where("head_repo_id = ? AND head_branch = ? AND has_merged = ? AND issue.is_closed = ?",
		repoID, branch, false, false).
		Join("INNER", "issue", "issue.id = pull_request.issue_id").Find(&prs)
}

// GetUnmergedPullRequestsByBaseInfo returns all pull requests that are open and has not been merged
// by given base information (repo and branch).
func GetUnmergedPullRequestsByBaseInfo(repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, x.Where("base_repo_id=? AND base_branch=? AND has_merged=? AND issue.is_closed=?",
		repoID, branch, false, false).
		Join("INNER", "issue", "issue.id=pull_request.issue_id").Find(&prs)
}

var _ errutil.NotFound = (*ErrPullRequestNotExist)(nil)

type ErrPullRequestNotExist struct {
	args map[string]interface{}
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
		return nil, ErrPullRequestNotExist{args: map[string]interface{}{"pullRequestID": id}}
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
		return nil, ErrPullRequestNotExist{args: map[string]interface{}{"issueID": issueID}}
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

// Update updates specific fields of pull request.
func (pr *PullRequest) UpdateCols(cols ...string) error {
	_, err := x.Id(pr.ID).Cols(cols...).Update(pr)
	return err
}

// UpdatePatch generates and saves a new patch.
func (pr *PullRequest) UpdatePatch() (err error) {
	headGitRepo, err := git.Open(pr.HeadRepo.RepoPath())
	if err != nil {
		return fmt.Errorf("open repository: %v", err)
	}

	// Add a temporary remote.
	tmpRemote := com.ToStr(time.Now().UnixNano())
	baseRepoPath := RepoPath(pr.BaseRepo.MustOwner().Name, pr.BaseRepo.Name)
	err = headGitRepo.RemoteAdd(tmpRemote, baseRepoPath, git.RemoteAddOptions{Fetch: true})
	if err != nil {
		return fmt.Errorf("add remote %q [repo_id: %d]: %v", tmpRemote, pr.HeadRepoID, err)
	}
	defer func() {
		if err := headGitRepo.RemoteRemove(tmpRemote); err != nil {
			log.Error("Failed to remove remote %q [repo_id: %d]: %v", tmpRemote, pr.HeadRepoID, err)
		}
	}()

	remoteBranch := "remotes/" + tmpRemote + "/" + pr.BaseBranch
	pr.MergeBase, err = headGitRepo.MergeBase(remoteBranch, pr.HeadBranch)
	if err != nil {
		return fmt.Errorf("get merge base: %v", err)
	} else if err = pr.Update(); err != nil {
		return fmt.Errorf("update: %v", err)
	}

	patch, err := headGitRepo.DiffBinary(pr.MergeBase, pr.HeadBranch)
	if err != nil {
		return fmt.Errorf("get binary patch: %v", err)
	}

	if err = pr.BaseRepo.SavePatch(pr.Index, patch); err != nil {
		return fmt.Errorf("save patch: %v", err)
	}

	log.Trace("PullRequest[%d].UpdatePatch: patch saved", pr.ID)
	return nil
}

// PushToBaseRepo pushes commits from branches of head repository to
// corresponding branches of base repository.
// FIXME: Only push branches that are actually updates?
func (pr *PullRequest) PushToBaseRepo() (err error) {
	log.Trace("PushToBaseRepo[%d]: pushing commits to base repo 'refs/pull/%d/head'", pr.BaseRepoID, pr.Index)

	headRepoPath := pr.HeadRepo.RepoPath()
	headGitRepo, err := git.Open(headRepoPath)
	if err != nil {
		return fmt.Errorf("open repository: %v", err)
	}

	tmpRemote := fmt.Sprintf("tmp-pull-%d", pr.ID)
	if err = headGitRepo.RemoteAdd(tmpRemote, pr.BaseRepo.RepoPath()); err != nil {
		return fmt.Errorf("add remote %q [repo_id: %d]: %v", tmpRemote, pr.HeadRepoID, err)
	}

	// Make sure to remove the remote even if the push fails
	defer func() {
		if err := headGitRepo.RemoteRemove(tmpRemote); err != nil {
			log.Error("Failed to remove remote %q [repo_id: %d]: %v", tmpRemote, pr.HeadRepoID, err)
		}
	}()

	headRefspec := fmt.Sprintf("refs/pull/%d/head", pr.Index)
	headFile := filepath.Join(pr.BaseRepo.RepoPath(), headRefspec)
	if osutil.IsExist(headFile) {
		err = os.Remove(headFile)
		if err != nil {
			return fmt.Errorf("remove head file [repo_id: %d]: %v", pr.BaseRepoID, err)
		}
	}

	err = headGitRepo.Push(tmpRemote, fmt.Sprintf("%s:%s", pr.HeadBranch, headRefspec))
	if err != nil {
		return fmt.Errorf("push: %v", err)
	}

	return nil
}

// AddToTaskQueue adds itself to pull request test task queue.
func (pr *PullRequest) AddToTaskQueue() {
	go PullRequestQueue.AddFunc(pr.ID, func() {
		pr.Status = PULL_REQUEST_STATUS_CHECKING
		if err := pr.UpdateCols("status"); err != nil {
			log.Error("AddToTaskQueue.UpdateCols[%d].(add to queue): %v", pr.ID, err)
		}
	})
}

type PullRequestList []*PullRequest

func (prs PullRequestList) loadAttributes(e Engine) (err error) {
	if len(prs) == 0 {
		return nil
	}

	// Load issues
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
		return fmt.Errorf("find issues: %v", err)
	}
	for i := range issues {
		set[issues[i].ID] = issues[i]
	}
	for i := range prs {
		prs[i].Issue = set[prs[i].IssueID]
	}

	// Load attributes
	for i := range prs {
		if err = prs[i].loadAttributes(e); err != nil {
			return fmt.Errorf("loadAttributes [%d]: %v", prs[i].ID, err)
		}
	}

	return nil
}

func (prs PullRequestList) LoadAttributes() error {
	return prs.loadAttributes(x)
}

func addHeadRepoTasks(prs []*PullRequest) {
	for _, pr := range prs {
		if pr.HeadRepo == nil {
			log.Trace("addHeadRepoTasks[%d]: missing head repository", pr.ID)
			continue
		}

		log.Trace("addHeadRepoTasks[%d]: composing new test task", pr.ID)
		if err := pr.UpdatePatch(); err != nil {
			log.Error("UpdatePatch: %v", err)
			continue
		} else if err := pr.PushToBaseRepo(); err != nil {
			log.Error("PushToBaseRepo: %v", err)
			continue
		}

		pr.AddToTaskQueue()
	}
}

// AddTestPullRequestTask adds new test tasks by given head/base repository and head/base branch,
// and generate new patch for testing as needed.
func AddTestPullRequestTask(doer *User, repoID int64, branch string, isSync bool) {
	log.Trace("AddTestPullRequestTask [head_repo_id: %d, head_branch: %s]: finding pull requests", repoID, branch)
	prs, err := GetUnmergedPullRequestsByHeadInfo(repoID, branch)
	if err != nil {
		log.Error("Find pull requests [head_repo_id: %d, head_branch: %s]: %v", repoID, branch, err)
		return
	}

	if isSync {
		if err = PullRequestList(prs).LoadAttributes(); err != nil {
			log.Error("PullRequestList.LoadAttributes: %v", err)
		}

		if err == nil {
			for _, pr := range prs {
				pr.Issue.PullRequest = pr
				if err = pr.Issue.LoadAttributes(); err != nil {
					log.Error("LoadAttributes: %v", err)
					continue
				}
				if err = PrepareWebhooks(pr.Issue.Repo, HOOK_EVENT_PULL_REQUEST, &api.PullRequestPayload{
					Action:      api.HOOK_ISSUE_SYNCHRONIZED,
					Index:       pr.Issue.Index,
					PullRequest: pr.Issue.PullRequest.APIFormat(),
					Repository:  pr.Issue.Repo.APIFormat(nil),
					Sender:      doer.APIFormat(),
				}); err != nil {
					log.Error("PrepareWebhooks [pull_id: %v]: %v", pr.ID, err)
					continue
				}
			}
		}
	}

	addHeadRepoTasks(prs)

	log.Trace("AddTestPullRequestTask [base_repo_id: %d, base_branch: %s]: finding pull requests", repoID, branch)
	prs, err = GetUnmergedPullRequestsByBaseInfo(repoID, branch)
	if err != nil {
		log.Error("Find pull requests [base_repo_id: %d, base_branch: %s]: %v", repoID, branch, err)
		return
	}
	for _, pr := range prs {
		pr.AddToTaskQueue()
	}
}

func ChangeUsernameInPullRequests(oldUserName, newUserName string) error {
	pr := PullRequest{
		HeadUserName: strings.ToLower(newUserName),
	}
	_, err := x.Cols("head_user_name").Where("head_user_name = ?", strings.ToLower(oldUserName)).Update(pr)
	return err
}

// checkAndUpdateStatus checks if pull request is possible to leaving checking status,
// and set to be either conflict or mergeable.
func (pr *PullRequest) checkAndUpdateStatus() {
	// Status is not changed to conflict means mergeable.
	if pr.Status == PULL_REQUEST_STATUS_CHECKING {
		pr.Status = PULL_REQUEST_STATUS_MERGEABLE
	}

	// Make sure there is no waiting test to process before leaving the checking status.
	if !PullRequestQueue.Exist(pr.ID) {
		if err := pr.UpdateCols("status"); err != nil {
			log.Error("Update[%d]: %v", pr.ID, err)
		}
	}
}

// TestPullRequests checks and tests untested patches of pull requests.
// TODO: test more pull requests at same time.
func TestPullRequests() {
	prs := make([]*PullRequest, 0, 10)
	_ = x.Iterate(PullRequest{
		Status: PULL_REQUEST_STATUS_CHECKING,
	},
		func(idx int, bean interface{}) error {
			pr := bean.(*PullRequest)

			if err := pr.LoadAttributes(); err != nil {
				log.Error("LoadAttributes: %v", err)
				return nil
			}

			if err := pr.testPatch(); err != nil {
				log.Error("testPatch: %v", err)
				return nil
			}
			prs = append(prs, pr)
			return nil
		})

	// Update pull request status.
	for _, pr := range prs {
		pr.checkAndUpdateStatus()
	}

	// Start listening on new test requests.
	for prID := range PullRequestQueue.Queue() {
		log.Trace("TestPullRequests[%v]: processing test task", prID)
		PullRequestQueue.Remove(prID)

		pr, err := GetPullRequestByID(com.StrTo(prID).MustInt64())
		if err != nil {
			log.Error("GetPullRequestByID[%s]: %v", prID, err)
			continue
		} else if err = pr.testPatch(); err != nil {
			log.Error("testPatch[%d]: %v", pr.ID, err)
			continue
		}

		pr.checkAndUpdateStatus()
	}
}

func InitTestPullRequests() {
	go TestPullRequests()
}
