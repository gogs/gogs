// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/gogits/git-module"
	"github.com/gogits/gogs/models"
)

var CmdPull = cli.Command{
	Name:        "pull",
	Usage:       "Filter commits for Pull Request actions",
	Description: `Checks commits for potential pull request updates and takes appropriate actions.`,
	Action:      runPull,
	Flags: []cli.Flag{
		stringFlag("path, p", "", "repository path"),
	},
}

func runPull(ctx *cli.Context) {
	setup("pull.log")

	if !ctx.IsSet("path") {
		log.Fatal("Missing argument --path")
	}

	workingDirectory := ctx.String("path")

	// Scan standard input (stdin) for updated refs
	stdin := bufio.NewScanner(os.Stdin)
	for stdin.Scan() {
		// Format from post-receive is: <old-commit> <new-commit> <ref-name>
		args := strings.Split(stdin.Text(), " ")
		if len(args) < 3 {
			continue
		}

		refName := args[2]
		refSplits := strings.Split(refName, "/")

		if len(refSplits) < 3 {
			log.Fatal("Not enough elements in refs element.")
		}

		//	if refSplits[1] == "pull" {
		//		log.Fatal("Not allowed to push to \"pull\" refs. Reserved for Pull Requests.")
		//	} else
		if refSplits[1] != "heads" {
			// Only push branches of ref "heads"
			continue
		}

		branch := strings.Join(refSplits[2:], "/")

		repoPathSplits := strings.Split(workingDirectory, string(os.PathSeparator))
		userName := repoPathSplits[len(repoPathSplits)-2]
		repoName := repoPathSplits[len(repoPathSplits)-1]
		repoName = repoName[0 : len(repoName)-4]

		pr, err := models.GetUnmergedPullRequestByRepoPathAndHeadBranch(userName, repoName, branch)
		if _, ok := err.(models.ErrPullRequestNotExist); ok {
			// Nothing to do here if the branch has no Pull Request open
			log.Printf("Skipping for %s/%s.git branch '%s'", userName, repoName, branch)
			continue
		} else if err != nil {
			log.Fatal("Database operation failed: " + err.Error())
		}

		err = pr.BaseRepo.GetOwner()
		if err != nil {
			log.Fatal("Could not get owner data: " + err.Error())
		}

		prIdStr := strconv.FormatInt(pr.ID, 10)
		tmpRemoteName := "tmp-pull-" + branch + "-" + prIdStr
		remoteUrl := "../../" + pr.BaseRepo.Owner.LowerName + "/" + pr.BaseRepo.LowerName + ".git"
		repo, err := git.OpenRepository(workingDirectory)
		repo.AddRemote(tmpRemoteName, remoteUrl, false)

		err = git.Push(workingDirectory, tmpRemoteName, branch+":"+"refs/pull/"+prIdStr+"/head")
		if err != nil {
			log.Fatal("Error pushing: " + err.Error())
		}

		err = repo.RemoveRemote(tmpRemoteName)

		if err != nil {
			log.Fatal("Error deleting temporary remote: " + err.Error())
		}
	}
}
