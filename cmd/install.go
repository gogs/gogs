// Copyright 2013 gopm authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package cmd

import (
	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdInstall = cli.Command{
	Name:  "install",
	Usage: "link dependencies and go install",
	Description: `Command install links dependencies according to gopmfile,
and execute 'go install'

gopm install
gopm install <import path>

If no argument is supplied, then gopmfile must be present`,
	Action: runInstall,
	Flags: []cli.Flag{
		cli.BoolFlag{"verbose", "show process details"},
	},
}

func runInstall(ctx *cli.Context) {
	var target string

	switch len(ctx.Args()) {
	case 0:
		if !com.IsFile(".gopmfile") {
			log.Fatal("Install", "No gopmfile exist in work directory")
		}

		gf := doc.NewGopmfile(".")
		target = gf.MustValue("target", "path")
		if len(target) == 0 {
			log.Fatal("Install", "Cannot find target in gopmfile")
		}
	case 1:
		target = ctx.Args()[0]
	default:
		log.Fatal("Install", "Too many arguments")
	}

	genNewGoPath(ctx, false)

	log.Trace("Installing...")

	cmdArgs := []string{"go", "install"}

	if ctx.Bool("verbose") {
		cmdArgs = append(cmdArgs, "-v")
	}
	cmdArgs = append(cmdArgs, target)
	err := execCmd(newGoPath, newCurPath, cmdArgs...)
	if err != nil {
		log.Error("Install", "Fail to install program")
		log.Fatal("", err.Error())
	}

	log.Success("SUCC", "Install", "Command execute successfully!")
}
