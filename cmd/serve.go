// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/uuid"
)

var CmdServ = cli.Command{
	Name:        "serv",
	Usage:       "This command should only be called by SSH shell",
	Description: `Serv provide access auth for repositories`,
	Action:      runServ,
	Flags:       []cli.Flag{},
}

func setup(logPath string) {
	setting.NewConfigContext()
	log.NewGitLogger(filepath.Join(setting.LogRootPath, logPath))

	if setting.DisableSSH {
		println("Gogs: SSH has been disabled")
		os.Exit(1)
	}

	models.LoadModelsConfig()

	if models.UseSQLite3 {
		workDir, _ := setting.WorkDir()
		os.Chdir(workDir)
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
	COMMANDS_READONLY = map[string]models.AccessType{
		"git-upload-pack":    models.WRITABLE,
		"git upload-pack":    models.WRITABLE,
		"git-upload-archive": models.WRITABLE,
	}

	COMMANDS_WRITE = map[string]models.AccessType{
		"git-receive-pack": models.READABLE,
		"git receive-pack": models.READABLE,
	}
)

func In(b string, sl map[string]models.AccessType) bool {
	_, e := sl[b]
	return e
}

func runServ(k *cli.Context) {
	setup("serv.log")

	keys := strings.Split(os.Args[2], "-")
	if len(keys) != 2 {
		println("Gogs: auth file format error")
		log.GitLogger.Fatal(2, "Invalid auth file format: %s", os.Args[2])
	}

	keyId, err := com.StrTo(keys[1]).Int64()
	if err != nil {
		println("Gogs: auth file format error")
		log.GitLogger.Fatal(2, "Invalid auth file format: %v", err)
	}
	user, err := models.GetUserByKeyId(keyId)
	if err != nil {
		if err == models.ErrUserNotKeyOwner {
			println("Gogs: you are not the owner of SSH key")
			log.GitLogger.Fatal(2, "Invalid owner of SSH key: %d", keyId)
		}
		println("Gogs: internal error:", err.Error())
		log.GitLogger.Fatal(2, "Fail to get user by key ID(%d): %v", keyId, err)
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
		log.GitLogger.Fatal(2, "Unavailable repository: %v", args)
	}
	repoUserName := rr[0]
	repoName := strings.TrimSuffix(rr[1], ".git")

	isWrite := In(verb, COMMANDS_WRITE)
	isRead := In(verb, COMMANDS_READONLY)

	repoUser, err := models.GetUserByName(repoUserName)
	if err != nil {
		if err == models.ErrUserNotExist {
			println("Gogs: given repository owner are not registered")
			log.GitLogger.Fatal(2, "Unregistered owner: %s", repoUserName)
		}
		println("Gogs: internal error:", err.Error())
		log.GitLogger.Fatal(2, "Fail to get repository owner(%s): %v", repoUserName, err)
	}

	// Access check.
	switch {
	case isWrite:
		has, err := models.HasAccess(user.Name, path.Join(repoUserName, repoName), models.WRITABLE)
		if err != nil {
			println("Gogs: internal error:", err.Error())
			log.GitLogger.Fatal(2, "Fail to check write access:", err)
		} else if !has {
			println("You have no right to write this repository")
			log.GitLogger.Fatal(2, "User %s has no right to write repository %s", user.Name, repoPath)
		}
	case isRead:
		repo, err := models.GetRepositoryByName(repoUser.Id, repoName)
		if err != nil {
			if err == models.ErrRepoNotExist {
				println("Gogs: given repository does not exist")
				log.GitLogger.Fatal(2, "Repository does not exist: %s/%s", repoUser.Name, repoName)
			}
			println("Gogs: internal error:", err.Error())
			log.GitLogger.Fatal(2, "Fail to get repository: %v", err)
		}

		if !repo.IsPrivate {
			break
		}

		has, err := models.HasAccess(user.Name, path.Join(repoUserName, repoName), models.READABLE)
		if err != nil {
			println("Gogs: internal error:", err.Error())
			log.GitLogger.Fatal(2, "Fail to check read access:", err)
		} else if !has {
			println("You have no right to access this repository")
			log.GitLogger.Fatal(2, "User %s has no right to read repository %s", user.Name, repoPath)
		}
	default:
		println("Unknown command: " + cmd)
		return
	}

	uuid := uuid.NewV4().String()
	os.Setenv("uuid", uuid)

	var gitcmd *exec.Cmd
	verbs := strings.Split(verb, " ")
	if len(verbs) == 2 {
		gitcmd = exec.Command(verbs[0], verbs[1], repoPath)
	} else {
		gitcmd = exec.Command(verb, repoPath)
	}
	gitcmd.Dir = setting.RepoRootPath
	gitcmd.Stdout = os.Stdout
	gitcmd.Stdin = os.Stdin
	gitcmd.Stderr = os.Stderr
	if err = gitcmd.Run(); err != nil {
		println("Gogs: internal error:", err.Error())
		log.GitLogger.Fatal(2, "Fail to execute git command: %v", err)
	}

	if isWrite {
		tasks, err := models.GetUpdateTasksByUuid(uuid)
		if err != nil {
			log.GitLogger.Fatal(2, "GetUpdateTasksByUuid: %v", err)
		}

		for _, task := range tasks {
			err = models.Update(task.RefName, task.OldCommitId, task.NewCommitId,
				user.Name, repoUserName, repoName, user.Id)
			if err != nil {
				log.GitLogger.Error(2, "Fail to update: %v", err)
			}
		}

		if err = models.DelUpdateTasksByUuid(uuid); err != nil {
			log.GitLogger.Fatal(2, "DelUpdateTasksByUuid: %v", err)
		}
	}

	// Update key activity.
	key, err := models.GetPublicKeyById(keyId)
	if err != nil {
		log.GitLogger.Fatal(2, "GetPublicKeyById: %v", err)
	}
	key.Updated = time.Now()
	if err = models.UpdatePublicKey(key); err != nil {
		log.GitLogger.Fatal(2, "UpdatePublicKey: %v", err)
	}
}
