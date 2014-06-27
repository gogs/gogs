// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"fmt"
	"os/exec"
	"strings"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func Update(refName, oldCommitId, newCommitId, userName, repoUserName, repoName string, userId int64) error {
	isNew := strings.HasPrefix(oldCommitId, "0000000")
	if isNew &&
		strings.HasPrefix(newCommitId, "0000000") {
		return fmt.Errorf("old rev and new rev both 000000")
	}

	f := RepoPath(repoUserName, repoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = f
	gitUpdate.Run()

	isDel := strings.HasPrefix(newCommitId, "0000000")
	if isDel {
		log.GitLogger.Info("del rev", refName, "from", userName+"/"+repoName+".git", "by", userId)
		return nil
	}

	repo, err := git.OpenRepository(f)
	if err != nil {
		return fmt.Errorf("runUpdate.Open repoId: %v", err)
	}

	newCommit, err := repo.GetCommit(newCommitId)
	if err != nil {
		return fmt.Errorf("runUpdate GetCommit of newCommitId: %v", err)
	}

	var l *list.List
	// if a new branch
	if isNew {
		l, err = newCommit.CommitsBefore()
		if err != nil {
			return fmt.Errorf("Find CommitsBefore erro: %v", err)
		}
	} else {
		l, err = newCommit.CommitsBeforeUntil(oldCommitId)
		if err != nil {
			return fmt.Errorf("Find CommitsBeforeUntil erro: %v", err)
		}
	}

	if err != nil {
		return fmt.Errorf("runUpdate.Commit repoId: %v", err)
	}

	ru, err := GetUserByName(repoUserName)
	if err != nil {
		return fmt.Errorf("runUpdate.GetUserByName: %v", err)
	}

	repos, err := GetRepositoryByName(ru.Id, repoName)
	if err != nil {
		return fmt.Errorf("runUpdate.GetRepositoryByName userId: %v", err)
	}

	commits := make([]*base.PushCommit, 0)
	var maxCommits = 3
	var actEmail string
	for e := l.Front(); e != nil; e = e.Next() {
		commit := e.Value.(*git.Commit)
		if actEmail == "" {
			actEmail = commit.Committer.Email
		}
		commits = append(commits,
			&base.PushCommit{commit.Id.String(),
				commit.Message(),
				commit.Author.Email,
				commit.Author.Name})
		if len(commits) >= maxCommits {
			break
		}
	}

	//commits = append(commits, []string{lastCommit.Id().String(), lastCommit.Message()})
	if err = CommitRepoAction(userId, ru.Id, userName, actEmail,
		repos.Id, repoUserName, repoName, refName, &base.PushCommits{l.Len(), commits}); err != nil {
		return fmt.Errorf("runUpdate.models.CommitRepoAction: %s/%s:%v", repoUserName, repoName, err)
	}
	return nil
}
