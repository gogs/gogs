// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	//"container/list"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
	qlog "github.com/qiniu/log"

	//"github.com/gogits/git"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
)

var (
	COMMANDS_READONLY = map[string]int{
		"git-upload-pack":    models.AU_WRITABLE,
		"git upload-pack":    models.AU_WRITABLE,
		"git-upload-archive": models.AU_WRITABLE,
	}

	COMMANDS_WRITE = map[string]int{
		"git-receive-pack": models.AU_READABLE,
		"git receive-pack": models.AU_READABLE,
	}
)

var CmdServ = cli.Command{
	Name:  "serv",
	Usage: "This command just should be called by ssh shell",
	Description: `
gogs serv provide access auth for repositories`,
	Action: runServ,
	Flags:  []cli.Flag{},
}

func newLogger(execDir string) {
	logPath := execDir + "/log/serv.log"
	os.MkdirAll(path.Dir(logPath), os.ModePerm)

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.ModePerm)
	if err != nil {
		qlog.Fatal(err)
	}

	qlog.SetOutput(f)
	qlog.Info("Start logging serv...")
}

func parseCmd(cmd string) (string, string) {
	ss := strings.SplitN(cmd, " ", 2)
	if len(ss) != 2 {
		return "", ""
	}

	verb, args := ss[0], ss[1]
	if verb == "git" {
		ss = strings.SplitN(args, " ", 2)
		args = ss[1]
		verb = fmt.Sprintf("%s %s", verb, ss[0])
	}
	return verb, args
}

func In(b string, sl map[string]int) bool {
	_, e := sl[b]
	return e
}

func runServ(k *cli.Context) {
	execDir, _ := base.ExecDir()
	newLogger(execDir)

	base.NewConfigContext()
	models.LoadModelsConfig()

	if models.UseSQLite3 {
		os.Chdir(execDir)
	}

	models.SetEngine()

	keys := strings.Split(os.Args[2], "-")
	if len(keys) != 2 {
		println("auth file format error")
		qlog.Fatal("auth file format error")
	}

	keyId, err := strconv.ParseInt(keys[1], 10, 64)
	if err != nil {
		println("auth file format error")
		qlog.Fatal("auth file format error", err)
	}
	user, err := models.GetUserByKeyId(keyId)
	if err != nil {
		println("You have no right to access")
		qlog.Fatalf("SSH visit error: %v", err)
	}

	cmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if cmd == "" {
		println("Hi", user.Name, "! You've successfully authenticated, but Gogs does not provide shell access.")
		return
	}

	verb, args := parseCmd(cmd)
	repoPath := strings.Trim(args, "'")
	rr := strings.SplitN(repoPath, "/", 2)
	if len(rr) != 2 {
		println("Unavailable repository", args)
		qlog.Fatalf("Unavailable repository %v", args)
	}
	repoUserName := rr[0]
	repoName := strings.TrimSuffix(rr[1], ".git")

	isWrite := In(verb, COMMANDS_WRITE)
	isRead := In(verb, COMMANDS_READONLY)

	repoUser, err := models.GetUserByName(repoUserName)
	if err != nil {
		println("You have no right to access")
		qlog.Fatal("Get user failed", err)
	}

	// access check
	switch {
	case isWrite:
		has, err := models.HasAccess(user.LowerName, path.Join(repoUserName, repoName), models.AU_WRITABLE)
		if err != nil {
			println("Internal error:", err)
			qlog.Fatal(err)
		} else if !has {
			println("You have no right to write this repository")
			qlog.Fatalf("User %s has no right to write repository %s", user.Name, repoPath)
		}
	case isRead:
		repo, err := models.GetRepositoryByName(repoUser.Id, repoName)
		if err != nil {
			println("Get repository error:", err)
			qlog.Fatal("Get repository error: " + err.Error())
		}

		if !repo.IsPrivate {
			break
		}

		has, err := models.HasAccess(user.Name, path.Join(repoUserName, repoName), models.AU_READABLE)
		if err != nil {
			println("Internal error")
			qlog.Fatal(err)
		}
		if !has {
			has, err = models.HasAccess(user.Name, repoPath, models.AU_WRITABLE)
			if err != nil {
				println("Internal error")
				qlog.Fatal(err)
			}
		}
		if !has {
			println("You have no right to access this repository")
			qlog.Fatal("You have no right to access this repository")
		}
	default:
		println("Unknown command")
		qlog.Fatal("Unknown command")
	}

	models.SetRepoEnvs(user.Id, user.Name, repoName)

	gitcmd := exec.Command(verb, repoPath)
	gitcmd.Dir = base.RepoRootPath
	gitcmd.Stdout = os.Stdout
	gitcmd.Stdin = os.Stdin
	gitcmd.Stderr = os.Stderr

	if err = gitcmd.Run(); err != nil {
		println("execute command error:", err.Error())
		qlog.Fatal("execute command error: " + err.Error())
	}

	//refName := os.Getenv("refName")
	//oldCommitId := os.Getenv("oldCommitId")
	//newCommitId := os.Getenv("newCommitId")

	//qlog.Error("get envs:", refName, oldCommitId, newCommitId)

	// update
	//models.Update(refName, oldCommitId, newCommitId, repoUserName, repoName, user.Id)
}
