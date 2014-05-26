// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/setting"
)

var CmdFix = cli.Command{
	Name:        "fix",
	Usage:       "This command for upgrade from old version",
	Description: `Fix provide upgrade from old version`,
	Action:      runFix,
	Flags:       []cli.Flag{},
}

func runFix(k *cli.Context) {
	workDir, _ := setting.WorkDir()
	newLogger(workDir)

	setting.NewConfigContext()
	models.LoadModelsConfig()

	if models.UseSQLite3 {
		os.Chdir(workDir)
	}

	models.SetEngine()

	err := models.Fix()
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Fix successfully!")
	}
}
