// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"
	"strconv"

	"github.com/codegangsta/cli"
	qlog "github.com/qiniu/log"

	"github.com/gogits/gogs/models"
)

var CmdUpdate = cli.Command{
	Name:        "update",
	Usage:       "This command should only be called by SSH shell",
	Description: `Update get pushed info and insert into database`,
	Action:      runUpdate,
	Flags:       []cli.Flag{},
}

func updateEnv(refName, oldCommitId, newCommitId string) {
	os.Setenv("refName", refName)
	os.Setenv("oldCommitId", oldCommitId)
	os.Setenv("newCommitId", newCommitId)
	qlog.Info("set envs:", refName, oldCommitId, newCommitId)
}

func runUpdate(c *cli.Context) {
	cmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if cmd == "" {
		return
	}

	setup("log/update.log")

	args := c.Args()
	if len(args) != 3 {
		qlog.Fatal("received less 3 parameters")
	} else if args[0] == "" {
		qlog.Fatal("refName is empty, shouldn't use")
	}

	//updateEnv(args[0], args[1], args[2])

	userName := os.Getenv("userName")
	userId, _ := strconv.ParseInt(os.Getenv("userId"), 10, 64)
	//repoId := os.Getenv("repoId")
	repoUserName := os.Getenv("repoUserName")
	repoName := os.Getenv("repoName")

	models.Update(args[0], args[1], args[2], userName, repoUserName, repoName, userId)
}
