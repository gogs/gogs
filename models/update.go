// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"os/exec"
	"strings"

	qlog "github.com/qiniu/log"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
)

func Update(refName, oldCommitId, newCommitId, userName, repoUserName, repoName string, userId int64) {
	isNew := strings.HasPrefix(oldCommitId, "0000000")
	if isNew &&
		strings.HasPrefix(newCommitId, "0000000") {
		qlog.Fatal("old rev and new rev both 000000")
	}

	f := RepoPath(repoUserName, repoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = f
	gitUpdate.Run()

	isDel := strings.HasPrefix(newCommitId, "0000000")
	if isDel {
		qlog.Info("del rev", refName, "from", userName+"/"+repoName+".git", "by", userId)
		return
	}

	repo, err := git.OpenRepository(f)
	if err != nil {
		qlog.Fatalf("runUpdate.Open repoId: %v", err)
	}

	newCommit, err := repo.GetCommit(newCommitId)
	if err != nil {
		qlog.Fatalf("runUpdate GetCommit of newCommitId: %v", err)
		return
	}

	var l *list.List
	// if a new branch
	if isNew {
		l, err = newCommit.CommitsBefore()
		if err != nil {
			qlog.Fatalf("Find CommitsBefore erro: %v", err)
		}
	} else {
		l, err = newCommit.CommitsBeforeUntil(oldCommitId)
		if err != nil {
			qlog.Fatalf("Find CommitsBeforeUntil erro: %v", err)
			return
		}
	}

	if err != nil {
		qlog.Fatalf("runUpdate.Commit repoId: %v", err)
	}

	ru, err := GetUserByName(repoUserName)
	if err != nil {
		qlog.Fatalf("runUpdate.GetUserByName: %v", err)
	}

	repos, err := GetRepositoryByName(ru.Id, repoName)
	if err != nil {
		qlog.Fatalf("runUpdate.GetRepositoryByName userId: %v", err)
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
		qlog.Fatalf("runUpdate.models.CommitRepoAction: %s/%s:%v", repoUserName, repoName, err)
	}
}
