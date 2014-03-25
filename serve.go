// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"container/list"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/codegangsta/cli"
	"github.com/gogits/gogs/modules/log"

	"github.com/gogits/git"
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

func init() {
	level := "0"
	os.MkdirAll("log", os.ModePerm)
	log.NewLogger(10000, "file", fmt.Sprintf(`{"level":%s,"filename":"%s"}`, level, "log/serv.log"))
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
	base.NewConfigContext()
	models.LoadModelsConfig()
	models.NewEngine()

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

	repo, err := models.GetRepositoryByName(user.Id, repoName)
	var isExist bool = true
	if err != nil {
		if err == models.ErrRepoNotExist {
			isExist = false
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
	}

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

	var rep *git.Repository
	repoPath := models.RepoPath(user.Name, repoName)
	if !isExist {
		if isWrite {
			_, err = models.CreateRepository(user, repoName, "", "", "", false, true)
			if err != nil {
				println("Create repository failed")
				log.Error("Create repository failed: " + err.Error())
				return
			}
		}
	}

	rep, err = git.OpenRepository(repoPath)
	if err != nil {
		println("OpenRepository failed:", err.Error())
		log.Error("OpenRepository failed: " + err.Error())
		return
	}

	refs, err := rep.AllReferencesMap()
	if err != nil {
		println("Get All References failed:", err.Error())
		log.Error("Get All References failed: " + err.Error())
		return
	}

	gitcmd := exec.Command(verb, rRepo)
	gitcmd.Dir = base.RepoRootPath

	var s string
	b := bytes.NewBufferString(s)

	gitcmd.Stdout = io.MultiWriter(os.Stdout, b)
	//gitcmd.Stdin = io.MultiReader(os.Stdin, b)
	gitcmd.Stdin = os.Stdin
	gitcmd.Stderr = os.Stderr

	if err = gitcmd.Run(); err != nil {
		println("execute command error:", err.Error())
		log.Error("execute command error: " + err.Error())
		return
	}

	if isRead {
		return
	}

	time.Sleep(time.Second)

	// find push reference name
	var t = "ok refs/heads/"
	var i int
	var refname string
	for {
		l, err := b.ReadString('\n')
		if err != nil {
			break
		}
		i = i + 1
		l = l[:len(l)-1]
		idx := strings.Index(l, t)
		if idx > 0 {
			refname = l[idx+len(t):]
		}
	}
	if refname == "" {
		println("No find any reference name:", b.String())
		log.Error("No find any reference name: " + b.String())
		return
	}

	var ref *git.Reference
	var ok bool
	var l *list.List
	//log.Info("----", refname, "-----")
	if ref, ok = refs[refname]; !ok {
		// for new branch
		refs, err = rep.AllReferencesMap()
		if err != nil {
			println("Get All References failed:", err.Error())
			log.Error("Get All References failed: " + err.Error())
			return
		}
		if ref, ok = refs[refname]; !ok {
			log.Error("unknow reference name -", refname, "-", b.String())
			log.Error("unknow reference name -", refname, "-", b.String())
			return
		}
		l, err = ref.AllCommits()
		if err != nil {
			println("Get All Commits failed:", err.Error())
			log.Error("Get All Commits failed: " + err.Error())
			return
		}
	} else {
		//log.Info("----", ref, "-----")
		var last *git.Commit
		//log.Info("00000", ref.Oid.String())
		last, err = ref.LastCommit()
		if err != nil {
			println("Get last commit failed:", err.Error())
			log.Error("Get last commit failed: " + err.Error())
			return
		}

		ref2, err := rep.LookupReference(ref.Name)
		if err != nil {
			println("look up reference failed:", err.Error())
			log.Error("look up reference failed: " + err.Error())
			return
		}

		//log.Info("11111", ref2.Oid.String())
		before, err := ref2.LastCommit()
		if err != nil {
			println("Get last commit failed:", err.Error())
			log.Error("Get last commit failed: " + err.Error())
			return
		}
		//log.Info("----", before.Id(), "-----", last.Id())
		l = ref.CommitsBetween(before, last)
	}

	commits := make([][]string, 0)
	var maxCommits = 3
	for e := l.Front(); e != nil; e = e.Next() {
		commit := e.Value.(*git.Commit)
		commits = append(commits, []string{commit.Id().String(), commit.Message()})
		if len(commits) >= maxCommits {
			break
		}
	}

	if err = models.CommitRepoAction(user.Id, user.Name,
		repo.Id, repoName, refname, &base.PushCommits{l.Len(), commits}); err != nil {
		log.Error("runUpdate.models.CommitRepoAction: %v", err, commits)
	} else {
		c := exec.Command("git", "update-server-info")
		c.Dir = repoPath
		err := c.Run()
		if err != nil {
			log.Error("update-server-info: %v", err)
		}
	}
}
