// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"fmt"
	"os/exec"
	"strings"

	git "github.com/gogs/git-module"
)

// CommitToPushCommit transforms a git.Commit to PushCommit type.
func CommitToPushCommit(commit *git.Commit) *PushCommit {
	return &PushCommit{
		Sha1:           commit.ID.String(),
		Message:        commit.Message(),
		AuthorEmail:    commit.Author.Email,
		AuthorName:     commit.Author.Name,
		CommitterEmail: commit.Committer.Email,
		CommitterName:  commit.Committer.Name,
		Timestamp:      commit.Committer.When,
	}
}

func ListToPushCommits(l *list.List) *PushCommits {
	if l == nil {
		return &PushCommits{}
	}

	commits := make([]*PushCommit, 0)
	var actEmail string
	for e := l.Front(); e != nil; e = e.Next() {
		commit := e.Value.(*git.Commit)
		if actEmail == "" {
			actEmail = commit.Committer.Email
		}
		commits = append(commits, CommitToPushCommit(commit))
	}
	return &PushCommits{l.Len(), commits, "", nil}
}

type PushUpdateOptions struct {
	OldCommitID  string
	NewCommitID  string
	RefFullName  string
	PusherID     int64
	PusherName   string
	RepoUserName string
	RepoName     string
}

// PushUpdate must be called for any push actions in order to
// generates necessary push action history feeds.
func PushUpdate(opts PushUpdateOptions) (err error) {
	isNewRef := opts.OldCommitID == git.EMPTY_SHA
	isDelRef := opts.NewCommitID == git.EMPTY_SHA
	if isNewRef && isDelRef {
		return fmt.Errorf("Old and new revisions are both %s", git.EMPTY_SHA)
	}

	repoPath := RepoPath(opts.RepoUserName, opts.RepoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = repoPath
	if err = gitUpdate.Run(); err != nil {
		return fmt.Errorf("Fail to call 'git update-server-info': %v", err)
	}

	gitRepo, err := git.OpenRepository(repoPath)
	if err != nil {
		return fmt.Errorf("OpenRepository: %v", err)
	}

	owner, err := GetUserByName(opts.RepoUserName)
	if err != nil {
		return fmt.Errorf("GetUserByName: %v", err)
	}

	repo, err := GetRepositoryByName(owner.ID, opts.RepoName)
	if err != nil {
		return fmt.Errorf("GetRepositoryByName: %v", err)
	}

	if err = repo.UpdateSize(); err != nil {
		return fmt.Errorf("UpdateSize: %v", err)
	}

	// Push tags
	if strings.HasPrefix(opts.RefFullName, git.TAG_PREFIX) {
		if err := CommitRepoAction(CommitRepoActionOptions{
			PusherName:  opts.PusherName,
			RepoOwnerID: owner.ID,
			RepoName:    repo.Name,
			RefFullName: opts.RefFullName,
			OldCommitID: opts.OldCommitID,
			NewCommitID: opts.NewCommitID,
			Commits:     &PushCommits{},
		}); err != nil {
			return fmt.Errorf("CommitRepoAction.(tag): %v", err)
		}
		return nil
	}

	var l *list.List
	// Skip read parent commits when delete branch
	if !isDelRef {
		// Push new branch
		newCommit, err := gitRepo.GetCommit(opts.NewCommitID)
		if err != nil {
			return fmt.Errorf("GetCommit [commit_id: %s]: %v", opts.NewCommitID, err)
		}

		if isNewRef {
			l, err = newCommit.CommitsBeforeLimit(10)
			if err != nil {
				return fmt.Errorf("CommitsBeforeLimit [commit_id: %s]: %v", newCommit.ID, err)
			}
		} else {
			l, err = newCommit.CommitsBeforeUntil(opts.OldCommitID)
			if err != nil {
				return fmt.Errorf("CommitsBeforeUntil [commit_id: %s]: %v", opts.OldCommitID, err)
			}
		}
	}

	if err := CommitRepoAction(CommitRepoActionOptions{
		PusherName:  opts.PusherName,
		RepoOwnerID: owner.ID,
		RepoName:    repo.Name,
		RefFullName: opts.RefFullName,
		OldCommitID: opts.OldCommitID,
		NewCommitID: opts.NewCommitID,
		Commits:     ListToPushCommits(l),
	}); err != nil {
		return fmt.Errorf("CommitRepoAction.(branch): %v", err)
	}
	return nil
}
