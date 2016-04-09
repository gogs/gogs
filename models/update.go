// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"fmt"
	"os/exec"
	"strings"

	git "github.com/gogits/git-module"

	"github.com/gogits/gogs/modules/log"
)

type UpdateTask struct {
	ID          int64  `xorm:"pk autoincr"`
	UUID        string `xorm:"index"`
	RefName     string
	OldCommitID string
	NewCommitID string
}

func AddUpdateTask(task *UpdateTask) error {
	_, err := x.Insert(task)
	return err
}

// GetUpdateTaskByUUID returns update task by given UUID.
func GetUpdateTaskByUUID(uuid string) (*UpdateTask, error) {
	task := &UpdateTask{
		UUID: uuid,
	}
	has, err := x.Get(task)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUpdateTaskNotExist{uuid}
	}
	return task, nil
}

func DeleteUpdateTaskByUUID(uuid string) error {
	_, err := x.Delete(&UpdateTask{UUID: uuid})
	return err
}

func ListToPushCommits(l *list.List) *PushCommits {
	commits := make([]*PushCommit, 0)
	var actEmail string
	for e := l.Front(); e != nil; e = e.Next() {
		commit := e.Value.(*git.Commit)
		if actEmail == "" {
			actEmail = commit.Committer.Email
		}
		commits = append(commits,
			&PushCommit{commit.ID.String(),
				commit.Message(),
				commit.Author.Email,
				commit.Author.Name,
			})
	}
	return &PushCommits{l.Len(), commits, "", nil}
}

type PushUpdateOptions struct {
	RefName      string
	OldCommitID  string
	NewCommitID  string
	PusherID     int64
	PusherName   string
	RepoUserName string
	RepoName     string
}

// PushUpdate must be called for any push actions in order to
// generates necessary push action history feeds.
func PushUpdate(opts PushUpdateOptions) (err error) {
	isNewRef := strings.HasPrefix(opts.OldCommitID, "0000000")
	isDelRef := strings.HasPrefix(opts.NewCommitID, "0000000")
	if isNewRef && isDelRef {
		return fmt.Errorf("Old and new revisions both start with 000000")
	}

	repoPath := RepoPath(opts.RepoUserName, opts.RepoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = repoPath
	if err = gitUpdate.Run(); err != nil {
		return fmt.Errorf("Fail to call 'git update-server-info': %v", err)
	}

	if isDelRef {
		log.GitLogger.Info("Reference '%s' has been deleted from '%s/%s' by %d",
			opts.RefName, opts.RepoUserName, opts.RepoName, opts.PusherName)
		return nil
	}

	gitRepo, err := git.OpenRepository(repoPath)
	if err != nil {
		return fmt.Errorf("OpenRepository: %v", err)
	}

	repoUser, err := GetUserByName(opts.RepoUserName)
	if err != nil {
		return fmt.Errorf("GetUserByName: %v", err)
	}

	repo, err := GetRepositoryByName(repoUser.Id, opts.RepoName)
	if err != nil {
		return fmt.Errorf("GetRepositoryByName: %v", err)
	}

	// Push tags.
	if strings.HasPrefix(opts.RefName, "refs/tags/") {
		tag, err := gitRepo.GetTag(git.RefEndName(opts.RefName))
		if err != nil {
			return fmt.Errorf("gitRepo.GetTag: %v", err)
		}

		// When tagger isn't available, fall back to get committer email.
		var actEmail string
		if tag.Tagger != nil {
			actEmail = tag.Tagger.Email
		} else {
			cmt, err := tag.Commit()
			if err != nil {
				return fmt.Errorf("tag.Commit: %v", err)
			}
			actEmail = cmt.Committer.Email
		}

		commit := &PushCommits{}
		if err = CommitRepoAction(opts.PusherID, repoUser.Id, opts.PusherName, actEmail,
			repo.ID, opts.RepoUserName, opts.RepoName, opts.RefName, commit, opts.OldCommitID, opts.NewCommitID); err != nil {
			return fmt.Errorf("CommitRepoAction (tag): %v", err)
		}
		return err
	}

	newCommit, err := gitRepo.GetCommit(opts.NewCommitID)
	if err != nil {
		return fmt.Errorf("gitRepo.GetCommit: %v", err)
	}

	// Push new branch.
	var l *list.List
	if isNewRef {
		l, err = newCommit.CommitsBeforeLimit(10)
		if err != nil {
			return fmt.Errorf("newCommit.CommitsBeforeLimit: %v", err)
		}
	} else {
		l, err = newCommit.CommitsBeforeUntil(opts.OldCommitID)
		if err != nil {
			return fmt.Errorf("newCommit.CommitsBeforeUntil: %v", err)
		}
	}

	if err = CommitRepoAction(opts.PusherID, repoUser.Id, opts.PusherName, repoUser.Email,
		repo.ID, opts.RepoUserName, opts.RepoName, opts.RefName, ListToPushCommits(l),
		opts.OldCommitID, opts.NewCommitID); err != nil {
		return fmt.Errorf("CommitRepoAction (branch): %v", err)
	}
	return nil
}
