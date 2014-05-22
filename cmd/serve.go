// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
	qlog "github.com/qiniu/log"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
)

var CmdServ = cli.Command{
	Name:        "serv",
	Usage:       "This command should only be called by SSH shell",
	Description: `Serv provide access auth for repositories`,
	Action:      runServ,
	Flags:       []cli.Flag{},
}

func newLogger(logPath string) {
	os.MkdirAll(path.Dir(logPath), os.ModePerm)

	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.ModePerm)
	if err != nil {
		qlog.Fatal(err)
	}

	qlog.SetOutput(f)
	//qlog.SetOutputLevel(qlog.Ldebug)
	qlog.Info("Start logging serv...")
}

func setup(logPath string) {
	execDir, _ := base.ExecDir()
	newLogger(path.Join(execDir, logPath))

	base.NewConfigContext()
	models.LoadModelsConfig()

	if models.UseSQLite3 {
		os.Chdir(execDir)
	}

	models.SetEngine()
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
	return verb, strings.Replace(args, "'/", "'", 1)
}

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

func In(b string, sl map[string]int) bool {
	_, e := sl[b]
	return e
}

func runServ(k *cli.Context) {
	setup("log/serv.log")

	keys := strings.Split(os.Args[2], "-")
	if len(keys) != 2 {
		println("Gogs: auth file format error")
		qlog.Fatal("Invalid auth file format: %s", os.Args[2])
	}

	keyId, err := strconv.ParseInt(keys[1], 10, 64)
	if err != nil {
		println("Gogs: auth file format error")
		qlog.Fatalf("Invalid auth file format: %v", err)
	}
	user, err := models.GetUserByKeyId(keyId)
	if err != nil {
		if err == models.ErrUserNotKeyOwner {
			println("Gogs: you are not the owner of SSH key")
			qlog.Fatalf("Invalid owner of SSH key: %d", keyId)
		}
		println("Gogs: internal error:", err)
		qlog.Fatalf("Fail to get user by key ID(%d): %v", keyId, err)
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
		println("Gogs: unavailable repository", args)
		qlog.Fatalf("Unavailable repository: %v", args)
	}
	repoUserName := rr[0]
	repoName := strings.TrimSuffix(rr[1], ".git")

	isWrite := In(verb, COMMANDS_WRITE)
	isRead := In(verb, COMMANDS_READONLY)

	repoUser, err := models.GetUserByName(repoUserName)
	if err != nil {
		if err == models.ErrUserNotExist {
			println("Gogs: given repository owner are not registered")
			qlog.Fatalf("Unregistered owner: %s", repoUserName)
		}
		println("Gogs: internal error:", err)
		qlog.Fatalf("Fail to get repository owner(%s): %v", repoUserName, err)
	}

	// Access check.
	switch {
	case isWrite:
		has, err := models.HasAccess(user.Name, path.Join(repoUserName, repoName), models.AU_WRITABLE)
		if err != nil {
			println("Gogs: internal error:", err)
			qlog.Fatal("Fail to check write access:", err)
		} else if !has {
			println("You have no right to write this repository")
			qlog.Fatalf("User %s has no right to write repository %s", user.Name, repoPath)
		}
	case isRead:
		repo, err := models.GetRepositoryByName(repoUser.Id, repoName)
		if err != nil {
			if err == models.ErrRepoNotExist {
				println("Gogs: given repository does not exist")
				qlog.Fatalf("Repository does not exist: %s/%s", repoUser.Name, repoName)
			}
			println("Gogs: internal error:", err)
			qlog.Fatalf("Fail to get repository: %v", err)
		}

		if !repo.IsPrivate {
			break
		}

		has, err := models.HasAccess(user.Name, path.Join(repoUserName, repoName), models.AU_READABLE)
		if err != nil {
			println("Gogs: internal error:", err)
			qlog.Fatal("Fail to check read access:", err)
		} else if !has {
			println("You have no right to access this repository")
			qlog.Fatalf("User %s has no right to read repository %s", user.Name, repoPath)
		}
	default:
		println("Unknown command")
		return
	}

	models.SetRepoEnvs(user.Id, user.Name, repoName, repoUserName)

	gitcmd := exec.Command(verb, repoPath)
	gitcmd.Dir = base.RepoRootPath
	gitcmd.Stdout = os.Stdout
	gitcmd.Stdin = os.Stdin
	gitcmd.Stderr = os.Stderr

	if err = gitcmd.Run(); err != nil {
		println("Gogs: internal error:", err)
		qlog.Fatalf("Fail to execute git command: %v", err)
	}

	//refName := os.Getenv("refName")
	//oldCommitId := os.Getenv("oldCommitId")
	//newCommitId := os.Getenv("newCommitId")

	//qlog.Error("get envs:", refName, oldCommitId, newCommitId)

	// update
	//models.Update(refName, oldCommitId, newCommitId, repoUserName, repoName, user.Id)
}
