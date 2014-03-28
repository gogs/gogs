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
	"github.com/gogits/gogs/modules/log"

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
	level := "0"
	logPath := execDir + "/log/serv.log"
	os.MkdirAll(path.Dir(logPath), os.ModePerm)
	log.NewLogger(10000, "file", fmt.Sprintf(`{"level":%s,"filename":"%s"}`, level, logPath))
	log.Trace("start logging...")
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
	log.Trace("new serv request " + log.Mode + ":" + log.Config)

	base.NewConfigContext()
	models.LoadModelsConfig()
	models.SetEngine()

	keys := strings.Split(os.Args[2], "-")
	if len(keys) != 2 {
		fmt.Println("auth file format error")
		log.Error("auth file format error")
		return
	}

	keyId, err := strconv.ParseInt(keys[1], 10, 64)
	if err != nil {
		fmt.Println("auth file format error")
		log.Error("auth file format error")
		return
	}
	user, err := models.GetUserByKeyId(keyId)
	if err != nil {
		fmt.Println("You have no right to access")
		log.Error("You have no right to access")
		return
	}

	cmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if cmd == "" {
		println("Hi", user.Name, "! You've successfully authenticated, but Gogs does not provide shell access.")
		return
	}

	verb, args := parseCmd(cmd)
	rRepo := strings.Trim(args, "'")
	rr := strings.SplitN(rRepo, "/", 2)
	if len(rr) != 2 {
		println("Unavilable repository", args)
		log.Error("Unavilable repository %v", args)
		return
	}
	repoName := rr[1]
	if strings.HasSuffix(repoName, ".git") {
		repoName = repoName[:len(repoName)-4]
	}

	isWrite := In(verb, COMMANDS_WRITE)
	isRead := In(verb, COMMANDS_READONLY)

	/*//repo, err := models.GetRepositoryByName(user.Id, repoName)
	//var isExist bool = true
	if err != nil {
		if err == models.ErrRepoNotExist {
			//isExist = false
			if isRead {
				println("Repository", user.Name+"/"+repoName, "is not exist")
				log.Error("Repository " + user.Name + "/" + repoName + " is not exist")
				return
			}
		} else {
			println("Get repository error:", err)
			log.Error("Get repository error: " + err.Error())
			return
		}
	}*/

	// access check
	switch {
	case isWrite:
		has, err := models.HasAccess(user.Name, repoName, models.AU_WRITABLE)
		if err != nil {
			println("Inernel error:", err)
			log.Error(err.Error())
			return
		}
		if !has {
			println("You have no right to write this repository")
			log.Error("You have no right to access this repository")
			return
		}
	case isRead:
		has, err := models.HasAccess(user.Name, repoName, models.AU_READABLE)
		if err != nil {
			println("Inernel error")
			log.Error(err.Error())
			return
		}
		if !has {
			has, err = models.HasAccess(user.Name, repoName, models.AU_WRITABLE)
			if err != nil {
				println("Inernel error")
				log.Error(err.Error())
				return
			}
		}
		if !has {
			println("You have no right to access this repository")
			log.Error("You have no right to access this repository")
			return
		}
	default:
		println("Unknown command")
		log.Error("Unknown command")
		return
	}

	// for update use
	os.Setenv("userName", user.Name)
	os.Setenv("userId", strconv.Itoa(int(user.Id)))
	os.Setenv("repoName", repoName)

	gitcmd := exec.Command(verb, rRepo)
	gitcmd.Dir = base.RepoRootPath
	gitcmd.Stdout = os.Stdout
	gitcmd.Stdin = os.Stdin
	gitcmd.Stderr = os.Stderr

	if err = gitcmd.Run(); err != nil {
		println("execute command error:", err.Error())
		log.Error("execute command error: " + err.Error())
		return
	}
}
