// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"
	"strings"
	"strconv"

	"github.com/urfave/cli"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/git-module"
)

var CmdUpdate = cli.Command{
	Name:        "update",
	Usage:       "This command should only be called by Git hook",
	Description: `Update get pushed info and insert into database`,
	Action:      runUpdate,
	Flags: []cli.Flag{
		stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
	},
}

func runUpdate(c *cli.Context) error {
	if c.IsSet("config") {
		setting.CustomConf = c.String("config")
	}

	setup("update.log")

	if len(os.Getenv("SSH_ORIGINAL_COMMAND")) == 0 {
		log.GitLogger.Trace("SSH_ORIGINAL_COMMAND is empty")
		return nil
	}

	args := c.Args()
	if len(args) != 3 {
		log.GitLogger.Fatal(2, "Arguments received are not equal to three")
	} else if len(args[0]) == 0 {
		log.GitLogger.Fatal(2, "First argument 'refName' is empty, shouldn't use")
	}

	branchName := strings.TrimPrefix(args[0], git.BRANCH_PREFIX)
	//UserID, _ := strconv.ParseInt(os.Getenv(models.PROTECTED_BRANCH_USER_ID), 10, 64)
	RepoID, _ := strconv.ParseInt(os.Getenv(models.PROTECTED_BRANCH_REPO_ID), 10, 64)
	accessMode := models.ParseAccessMode(os.Getenv(models.PROTECTED_BRANCH_ACCESS_MODE))
	//skip admin or owner AccessMode
	if (accessMode == models.ACCESS_MODE_WRITE) {
		if protectBranch, err := models.GetProtectedBranchBy(RepoID, branchName); err == nil {
			if (protectBranch != nil && !protectBranch.CanPush) {
				log.GitLogger.Fatal(2, "Protected Branch Cann't Push")
			}
		}
	}
	task := models.UpdateTask{
		UUID:        os.Getenv("uuid"),
		RefName:     args[0],
		OldCommitID: args[1],
		NewCommitID: args[2],
	}

	if err := models.AddUpdateTask(&task); err != nil {
		log.GitLogger.Fatal(2, "AddUpdateTask: %v", err)
	}

	return nil
}
