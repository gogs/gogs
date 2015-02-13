// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"

	"github.com/codegangsta/cli"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

var CmdUpdate = cli.Command{
	Name:        "update",
	Usage:       "This command should only be called by SSH shell",
	Description: `Update get pushed info and insert into database`,
	Action:      runUpdate,
	Flags: []cli.Flag{
		cli.StringFlag{"config, c", "custom/conf/app.ini", "Custom configuration file path", ""},
	},
}

func runUpdate(c *cli.Context) {
	if c.IsSet("config") {
		setting.CustomConf = c.String("config")
	}
	cmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if cmd == "" {
		return
	}

	setup("update.log")

	args := c.Args()
	if len(args) != 3 {
		log.GitLogger.Fatal(2, "received less 3 parameters")
	} else if args[0] == "" {
		log.GitLogger.Fatal(2, "refName is empty, shouldn't use")
	}

	uuid := os.Getenv("uuid")

	task := models.UpdateTask{
		Uuid:        uuid,
		RefName:     args[0],
		OldCommitId: args[1],
		NewCommitId: args[2],
	}

	if err := models.AddUpdateTask(&task); err != nil {
		log.GitLogger.Fatal(2, err.Error())
	}
}
