// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"strconv"

	"github.com/codegangsta/cli"

	"github.com/gogits/git"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
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
func runUpdate(*cli.Context) {
	userName := os.Getenv("userName")
	userId := os.Getenv("userId")
	repoId := os.Getenv("repoId")
	repoName := os.Getenv("repoName")

	f := models.RepoPath(userName, repoName)

	repo, err := git.OpenRepository(f)
	if err != nil {
		return
	}

	ref, err := repo.LookupReference("HEAD")
	if err != nil {
		return
	}

	lastCommit, err := repo.LookupCommit(ref.Oid)
	if err != nil {
		return
	}

	sUserId, err := strconv.Atoi(userId)
	if err != nil {
		log.Error("runUpdate.Parse userId: %v", err)
		return
	}
	sRepoId, err := strconv.Atoi(repoId)
	if err != nil {
		log.Error("runUpdate.Parse repoId: %v", err)
		return
	}
	commits := make([][]string, 0)
	commits = append(commits, []string{lastCommit.Id().String(), lastCommit.Message()})
	if err = models.CommitRepoAction(int64(sUserId), userName,
		int64(sRepoId), repoName, commits); err != nil {
		log.Error("runUpdate.models.CommitRepoAction: %v", err)
	}
}
