// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"fmt"
	"os/exec"
	"strings"

	git "github.com/gogits/git-shell"

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

func Update(refName, oldCommitID, newCommitID, userName, repoUserName, repoName string, userID int64) error {
	isNew := strings.HasPrefix(oldCommitID, "0000000")
	if isNew &&
		strings.HasPrefix(newCommitID, "0000000") {
		return fmt.Errorf("old rev and new rev both 000000")
	}

	f := RepoPath(repoUserName, repoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = f
	gitUpdate.Run()

	isDel := strings.HasPrefix(newCommitID, "0000000")
	if isDel {
		log.GitLogger.Info("del rev", refName, "from", userName+"/"+repoName+".git", "by", userID)
		return nil
	}

	gitRepo, err := git.OpenRepository(f)
	if err != nil {
		return fmt.Errorf("runUpdate.Open repoId: %v", err)
	}

	user, err := GetUserByName(repoUserName)
	if err != nil {
		return fmt.Errorf("runUpdate.GetUserByName: %v", err)
	}

	repo, err := GetRepositoryByName(user.Id, repoName)
	if err != nil {
		return fmt.Errorf("runUpdate.GetRepositoryByName userId: %v", err)
	}

	// Push tags.
	if strings.HasPrefix(refName, "refs/tags/") {
		tagName := git.RefEndName(refName)
		tag, err := gitRepo.GetTag(tagName)
		if err != nil {
			log.GitLogger.Fatal(4, "runUpdate.GetTag: %v", err)
		}

		var actEmail string
		if tag.Tagger != nil {
			actEmail = tag.Tagger.Email
		} else {
			cmt, err := tag.Commit()
			if err != nil {
				log.GitLogger.Fatal(4, "runUpdate.GetTag Commit: %v", err)
			}
			actEmail = cmt.Committer.Email
		}

		commit := &PushCommits{}

		if err = CommitRepoAction(userID, user.Id, userName, actEmail,
			repo.ID, repoUserName, repoName, refName, commit, oldCommitID, newCommitID); err != nil {
			log.GitLogger.Fatal(4, "CommitRepoAction: %s/%s:%v", repoUserName, repoName, err)
		}
		return err
	}

	newCommit, err := gitRepo.GetCommit(newCommitID)
	if err != nil {
		return fmt.Errorf("runUpdate GetCommit of newCommitId: %v", err)
	}

	// Push new branch.
	var l *list.List
	if isNew {
		l, err = newCommit.CommitsBeforeLimit(10)
		if err != nil {
			return fmt.Errorf("CommitsBefore: %v", err)
		}
	} else {
		l, err = newCommit.CommitsBeforeUntil(oldCommitID)
		if err != nil {
			return fmt.Errorf("CommitsBeforeUntil: %v", err)
		}
	}

	if err != nil {
		return fmt.Errorf("runUpdate.Commit repoId: %v", err)
	}

	if err = CommitRepoAction(userID, user.Id, userName, user.Email,
		repo.ID, repoUserName, repoName, refName, ListToPushCommits(l), oldCommitID, newCommitID); err != nil {
		return fmt.Errorf("runUpdate.models.CommitRepoAction: %s/%s:%v", repoUserName, repoName, err)
	}
	return nil
}
