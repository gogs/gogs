// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path"
	"strconv"

	"github.com/codegangsta/cli"
	qlog "github.com/qiniu/log"

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

func updateEnv(refName, oldCommitId, newCommitId string) {
	os.Setenv("refName", refName)
	os.Setenv("oldCommitId", oldCommitId)
	os.Setenv("newCommitId", newCommitId)
	qlog.Error("set envs:", refName, oldCommitId, newCommitId)
}

// for command: ./gogs update
func runUpdate(c *cli.Context) {
	cmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if cmd == "" {
		return
	}

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

	if args[0] == "" {
		qlog.Fatal("refName is empty, shouldn't use")
	}

	//updateEnv(args[0], args[1], args[2])

	userName := os.Getenv("userName")
	userId := os.Getenv("userId")
	iUserId, _ := strconv.ParseInt(userId, 10, 64)
	//repoId := os.Getenv("repoId")
	repoName := os.Getenv("repoName")

	models.Update(args[0], args[1], args[2], userName, repoName, iUserId)
}
