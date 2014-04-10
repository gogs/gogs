// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
	qlog "github.com/qiniu/log"

	"github.com/gogits/git"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
)

var CmdUpdate = cli.Command{
	Name:  "update",
	Usage: "This command just should be called by ssh shell",
	Description: `
gogs serv provide access auth for repositories`,
	Action: runUpdate,
	Flags:  []cli.Flag{},
}

func newUpdateLogger(execDir string) {
	logPath := execDir + "/log/update.log"
	os.MkdirAll(path.Dir(logPath), os.ModePerm)

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.ModePerm)
	if err != nil {
		qlog.Fatal(err)
	}

	qlog.SetOutput(f)
	qlog.Info("Start logging update...")
}

// for command: ./gogs update
func runUpdate(c *cli.Context) {
	execDir, _ := base.ExecDir()
	newUpdateLogger(execDir)

	base.NewConfigContext()
	models.LoadModelsConfig()

	if models.UseSQLite3 {
		os.Chdir(execDir)
	}

	models.SetEngine()

	args := c.Args()
	if len(args) != 3 {
		qlog.Fatal("received less 3 parameters")
	}

	refName := args[0]
	if refName == "" {
		qlog.Fatal("refName is empty, shouldn't use")
	}
	oldCommitId := args[1]
	newCommitId := args[2]

	isNew := strings.HasPrefix(oldCommitId, "0000000")
	if isNew &&
		strings.HasPrefix(newCommitId, "0000000") {
		qlog.Fatal("old rev and new rev both 000000")
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
		qlog.Fatalf("runUpdate.Open repoId: %v", err)
	}

	newOid, err := git.NewOidFromString(newCommitId)
	if err != nil {
		qlog.Fatalf("runUpdate.Ref repoId: %v", err)
	}

	newCommit, err := repo.LookupCommit(newOid)
	if err != nil {
		qlog.Fatalf("runUpdate.Ref repoId: %v", err)
	}

	var l *list.List
	// if a new branch
	if isNew {
		l, err = repo.CommitsBefore(newCommit.Id())
		if err != nil {
			qlog.Fatalf("Find CommitsBefore erro:", err)
		}
	} else {
		oldOid, err := git.NewOidFromString(oldCommitId)
		if err != nil {
			qlog.Fatalf("runUpdate.Ref repoId: %v", err)
		}

		oldCommit, err := repo.LookupCommit(oldOid)
		if err != nil {
			qlog.Fatalf("runUpdate.Ref repoId: %v", err)
		}
		l = repo.CommitsBetween(newCommit, oldCommit)
	}

	if err != nil {
		qlog.Fatalf("runUpdate.Commit repoId: %v", err)
	}

	sUserId, err := strconv.Atoi(userId)
	if err != nil {
		qlog.Fatalf("runUpdate.Parse userId: %v", err)
	}

	repos, err := models.GetRepositoryByName(int64(sUserId), repoName)
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
		qlog.Fatalf("runUpdate.models.CommitRepoAction: %v", err)
	}
}
