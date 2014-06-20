// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"os/exec"
	"strings"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func Update(refName, oldCommitId, newCommitId, userName, repoUserName, repoName string, userId int64) {
	isNew := strings.HasPrefix(oldCommitId, "0000000")
	if isNew &&
		strings.HasPrefix(newCommitId, "0000000") {
		log.GitLogger.Fatal("old rev and new rev both 000000")
	}

	f := RepoPath(repoUserName, repoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = f
	gitUpdate.Run()

	isDel := strings.HasPrefix(newCommitId, "0000000")
	if isDel {
		log.GitLogger.Info("del rev", refName, "from", userName+"/"+repoName+".git", "by", userId)
		return
	}

	repo, err := git.OpenRepository(f)
	if err != nil {
		log.GitLogger.Fatal("runUpdate.Open repoId: %v", err)
	}

	newCommit, err := repo.GetCommit(newCommitId)
	if err != nil {
		log.GitLogger.Fatal("runUpdate GetCommit of newCommitId: %v", err)
		return
	}

	var l *list.List
	// if a new branch
	if isNew {
		l, err = newCommit.CommitsBefore()
		if err != nil {
			log.GitLogger.Fatal("Find CommitsBefore erro: %v", err)
		}
	} else {
		l, err = newCommit.CommitsBeforeUntil(oldCommitId)
		if err != nil {
			log.GitLogger.Fatal("Find CommitsBeforeUntil erro: %v", err)
			return
		}
	}

	if err != nil {
		log.GitLogger.Fatal("runUpdate.Commit repoId: %v", err)
	}

	ru, err := GetUserByName(repoUserName)
	if err != nil {
		log.GitLogger.Fatal("runUpdate.GetUserByName: %v", err)
	}

	repos, err := GetRepositoryByName(ru.Id, repoName)
	if err != nil {
		log.GitLogger.Fatal("runUpdate.GetRepositoryByName userId: %v", err)
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
		log.GitLogger.Fatal("runUpdate.models.CommitRepoAction: %s/%s:%v", repoUserName, repoName, err)
	}
}
