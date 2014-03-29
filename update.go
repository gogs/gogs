// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
	//"github.com/gogits/gogs/modules/log"
	"github.com/gogits/git"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/qiniu/log"
)

var CmdUpdate = cli.Command{
	Name:  "update",
	Usage: "This command just should be called by ssh shell",
	Description: `
gogs serv provide access auth for repositories`,
	Action: runUpdate,
	Flags:  []cli.Flag{},
}

// for command: ./gogs update
func runUpdate(c *cli.Context) {
	base.NewConfigContext()
	models.LoadModelsConfig()
	models.SetEngine()

	w, _ := os.Create("update.log")
	defer w.Close()

	log.SetOutput(w)

	args := c.Args()
	//log.Info(args)
	if len(args) != 3 {
		log.Error("received less 3 parameters")
		return
	}

	refName := args[0]
	if refName == "" {
		log.Error("refName is empty, shouldn't use")
		return
	}
	oldCommitId := args[1]
	newCommitId := args[2]

	isNew := strings.HasPrefix(oldCommitId, "0000000")
	if isNew &&
		strings.HasPrefix(newCommitId, "0000000") {
		log.Error("old rev and new rev both 000000")
		return
	}

	userName := os.Getenv("userName")
	userId := os.Getenv("userId")
	//repoId := os.Getenv("repoId")
	repoName := os.Getenv("repoName")

	f := models.RepoPath(userName, repoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = f
	gitUpdate.Run()

	repo, err := git.OpenRepository(f)
	if err != nil {
		log.Error("runUpdate.Open repoId: %v", err)
		return
	}

	newOid, err := git.NewOidFromString(newCommitId)
	if err != nil {
		log.Error("runUpdate.Ref repoId: %v", err)
		return
	}

	newCommit, err := repo.LookupCommit(newOid)
	if err != nil {
		log.Error("runUpdate.Ref repoId: %v", err)
		return
	}

	var l *list.List
	// if a new branch
	if isNew {
		l, err = repo.CommitsBefore(newCommit.Id())
		if err != nil {
			log.Error("Find CommitsBefore erro:", err)
			return
		}
	} else {
		oldOid, err := git.NewOidFromString(oldCommitId)
		if err != nil {
			log.Error("runUpdate.Ref repoId: %v", err)
			return
		}

		oldCommit, err := repo.LookupCommit(oldOid)
		if err != nil {
			log.Error("runUpdate.Ref repoId: %v", err)
			return
		}
		l = repo.CommitsBetween(newCommit, oldCommit)
	}

	if err != nil {
		log.Error("runUpdate.Commit repoId: %v", err)
		return
	}

	sUserId, err := strconv.Atoi(userId)
	if err != nil {
		log.Error("runUpdate.Parse userId: %v", err)
		return
	}

	repos, err := models.GetRepositoryByName(int64(sUserId), repoName)
	if err != nil {
		log.Error("runUpdate.GetRepositoryByName userId: %v", err)
		return
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
			&base.PushCommit{commit.Id().String(),
				commit.Message(),
				commit.Author.Email,
				commit.Author.Name})
		if len(commits) >= maxCommits {
			break
		}
	}

	//commits = append(commits, []string{lastCommit.Id().String(), lastCommit.Message()})
	if err = models.CommitRepoAction(int64(sUserId), userName, actEmail,
		repos.Id, repoName, git.BranchName(refName), &base.PushCommits{l.Len(), commits}); err != nil {
		log.Error("runUpdate.models.CommitRepoAction: %v", err)
	}
}
