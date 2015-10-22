// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/process"
)

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
	ID     int64 `xorm:"pk autoincr"`
	Type   PullRequestType
	Status PullRequestStatus

	IssueID int64  `xorm:"INDEX"`
	Issue   *Issue `xorm:"-"`
	Index   int64

	HeadRepoID     int64
	HeadRepo       *Repository `xorm:"-"`
	BaseRepoID     int64
	HeadUserName   string
	HeadBranch     string
	BaseBranch     string
	MergeBase      string `xorm:"VARCHAR(40)"`
	MergedCommitID string `xorm:"VARCHAR(40)"`

	HasMerged bool
	Merged    time.Time
	MergerID  int64
	Merger    *User `xorm:"-"`
}

// Note: don't try to get Pull because will end up recursive querying.
func (pr *PullRequest) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "head_repo_id":
		// FIXME: shouldn't show error if it's known that head repository has been removed.
		pr.HeadRepo, err = GetRepositoryByID(pr.HeadRepoID)
		if err != nil {
			log.Error(3, "GetRepositoryByID[%d]: %v", pr.ID, err)
		}
	case "merger_id":
		if !pr.HasMerged {
			return
		}

		pr.Merger, err = GetUserByID(pr.MergerID)
		if err != nil {
			if IsErrUserNotExist(err) {
				pr.MergerID = -1
				pr.Merger = NewFakeUser()
			} else {
				log.Error(3, "GetUserByID[%d]: %v", pr.ID, err)
			}
		}
	case "merged":
		if !pr.HasMerged {
			return
		}

		pr.Merged = regulateTimeZone(pr.Merged)
	}
}

// CanAutoMerge returns true if this pull request can be merged automatically.
func (pr *PullRequest) CanAutoMerge() bool {
	return pr.Status == PULL_REQUEST_STATUS_MERGEABLE
}

// Merge merges pull request to base repository.
func (pr *PullRequest) Merge(doer *User, baseGitRepo *git.Repository) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = pr.Issue.changeStatus(sess, doer, true); err != nil {
		return fmt.Errorf("Pull.changeStatus: %v", err)
	}

	headRepoPath := RepoPath(pr.HeadUserName, pr.HeadRepo.Name)
	headGitRepo, err := git.OpenRepository(headRepoPath)
	if err != nil {
		return fmt.Errorf("OpenRepository: %v", err)
	}
	pr.MergedCommitID, err = headGitRepo.GetCommitIdOfBranch(pr.HeadBranch)
	if err != nil {
		return fmt.Errorf("GetCommitIdOfBranch: %v", err)
	}

	if err = mergePullRequestAction(sess, doer, pr.Issue.Repo, pr.Issue); err != nil {
		return fmt.Errorf("mergePullRequestAction: %v", err)
	}

	pr.HasMerged = true
	pr.Merged = time.Now()
	pr.MergerID = doer.Id
	if _, err = sess.Id(pr.ID).AllCols().Update(pr); err != nil {
		return fmt.Errorf("update pull request: %v", err)
	}

	// Clone base repo.
	tmpBasePath := path.Join("data/tmp/repos", com.ToStr(time.Now().Nanosecond())+".git")
	os.MkdirAll(path.Dir(tmpBasePath), os.ModePerm)
	defer os.RemoveAll(path.Dir(tmpBasePath))

	var stderr string
	if _, stderr, err = process.ExecTimeout(5*time.Minute,
		fmt.Sprintf("PullRequest.Merge(git clone): %s", tmpBasePath),
		"git", "clone", baseGitRepo.Path, tmpBasePath); err != nil {
		return fmt.Errorf("git clone: %s", stderr)
	}

	// Check out base branch.
	if _, stderr, err = process.ExecDir(-1, tmpBasePath,
		fmt.Sprintf("PullRequest.Merge(git checkout): %s", tmpBasePath),
		"git", "checkout", pr.BaseBranch); err != nil {
		return fmt.Errorf("git checkout: %s", stderr)
	}

	// Pull commits.
	if _, stderr, err = process.ExecDir(-1, tmpBasePath,
		fmt.Sprintf("PullRequest.Merge(git pull): %s", tmpBasePath),
		"git", "pull", headRepoPath, pr.HeadBranch); err != nil {
		return fmt.Errorf("git pull[%s / %s -> %s]: %s", headRepoPath, pr.HeadBranch, tmpBasePath, stderr)
	}

	// Push back to upstream.
	if _, stderr, err = process.ExecDir(-1, tmpBasePath,
		fmt.Sprintf("PullRequest.Merge(git push): %s", tmpBasePath),
		"git", "push", baseGitRepo.Path, pr.BaseBranch); err != nil {
		return fmt.Errorf("git push: %s", stderr)
	}

	return sess.Commit()
}

// NewPullRequest creates new pull request with labels for repository.
func NewPullRequest(repo *Repository, pull *Issue, labelIDs []int64, uuids []string, pr *PullRequest, patch []byte) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssue(sess, repo, pull, labelIDs, uuids, true); err != nil {
		return fmt.Errorf("newIssue: %v", err)
	}

	// Notify watchers.
	act := &Action{
		ActUserID:    pull.Poster.Id,
		ActUserName:  pull.Poster.Name,
		ActEmail:     pull.Poster.Email,
		OpType:       CREATE_PULL_REQUEST,
		Content:      fmt.Sprintf("%d|%s", pull.Index, pull.Name),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	}
	if err = notifyWatchers(sess, act); err != nil {
		return err
	}

	// Test apply patch.
	if err = repo.UpdateLocalCopy(); err != nil {
		return fmt.Errorf("UpdateLocalCopy: %v", err)
	}

	repoPath, err := repo.RepoPath()
	if err != nil {
		return fmt.Errorf("RepoPath: %v", err)
	}
	patchPath := path.Join(repoPath, "pulls", com.ToStr(pull.ID)+".patch")

	os.MkdirAll(path.Dir(patchPath), os.ModePerm)
	if err = ioutil.WriteFile(patchPath, patch, 0644); err != nil {
		return fmt.Errorf("save patch: %v", err)
	}

	pr.Status = PULL_REQUEST_STATUS_MERGEABLE
	_, stderr, err := process.ExecDir(-1, repo.LocalCopyPath(),
		fmt.Sprintf("NewPullRequest(git apply --check): %d", repo.ID),
		"git", "apply", "--check", patchPath)
	if err != nil {
		if strings.Contains(stderr, "patch does not apply") {
			pr.Status = PULL_REQUEST_STATUS_CONFLICT
		} else {
			return fmt.Errorf("git apply --check: %v - %s", err, stderr)
		}
	}

	pr.IssueID = pull.ID
	pr.Index = pull.Index
	if _, err = sess.Insert(pr); err != nil {
		return fmt.Errorf("insert pull repo: %v", err)
	}

	return sess.Commit()
}

// GetUnmergedPullRequest returnss a pull request that is open and has not been merged
// by given head/base and repo/branch.
func GetUnmergedPullRequest(headRepoID, baseRepoID int64, headBranch, baseBranch string) (*PullRequest, error) {
	pr := new(PullRequest)

	has, err := x.Where("head_repo_id=? AND head_branch=? AND base_repo_id=? AND base_branch=? AND has_merged=? AND issue.is_closed=?",
		headRepoID, headBranch, baseRepoID, baseBranch, false, false).
		Join("INNER", "issue", "issue.id=pull_request.issue_id").Get(pr)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrPullRequestNotExist{0, 0, headRepoID, baseRepoID, headBranch, baseBranch}
	}

	return pr, nil
}

// GetPullRequestByIssueID returns pull request by given issue ID.
func GetPullRequestByIssueID(issueID int64) (*PullRequest, error) {
	pr := &PullRequest{
		IssueID: issueID,
	}
	has, err := x.Get(pr)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrPullRequestNotExist{0, issueID, 0, 0, "", ""}
	}
	return pr, nil
}
