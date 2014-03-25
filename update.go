// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
"os"
"github.com/codegangsta/cli"
//"github.com/gogits/gogs/modules/log"
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
	//level := "0"
	//os.MkdirAll("log", os.ModePerm)
	//log.NewLogger(10000, "file", fmt.Sprintf(`{"level":%s,"filename":"%s"}`, level, "log/serv.log"))
	//log.Info("start update logging...")

	w, _ := os.Create("update.log")
	defer w.Close()

	log.SetOutput(w)
	for i, arg := range c.Args() {
		log.Info(i, arg)
	}
	userName := os.Getenv("userName")
	//userId := os.Getenv("userId")
	//repoId := os.Getenv("repoId")
	//repoName := os.Getenv("repoName")

	log.Info("username", userName)
	/*f := models.RepoPath(userName, repoName)

	repo, err := git.OpenRepository(f)
	if err != nil {
		log.Error("runUpdate.Open repoId: %v", err)
		return
	}

	ref, err := repo.LookupReference("HEAD")
	if err != nil {
		log.Error("runUpdate.Ref repoId: %v", err)
		return
	}

	lastCommit, err := repo.LookupCommit(ref.Oid)
	if err != nil {
		log.Error("runUpdate.Commit repoId: %v", err)
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
	} else {
		l := exec.Command("exec", "git", "update-server-info")
		l.Run()
	}*/
}
