package main

import (
	"os"
	"strconv"

	"github.com/gogits/gogs/models"

	"github.com/codegangsta/cli"
	git "github.com/gogits/git"
)

var CmdUpdate = cli.Command{
	Name:  "update",
	Usage: "This command just should be called by ssh shell",
	Description: `
gogs serv provide access auth for repositories`,
	Action: runUpdate,
	Flags:  []cli.Flag{},
}

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
	sUserId, _ := strconv.Atoi(userId)
	sRepoId, _ := strconv.Atoi(repoId)
	err = models.CommitRepoAction(int64(sUserId), userName,
		int64(sRepoId), repoName, lastCommit.Message())
	if err != nil {
		//TODO: log
	}
}
