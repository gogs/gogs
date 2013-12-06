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
	"path/filepath"

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
		cli.BoolFlag{"pkg, p", "only install non-main packages"},
		cli.BoolFlag{"verbose, v", "show process details"},
	},
}

func runInstall(ctx *cli.Context) {
	setup(ctx)

	var target string
	switch len(ctx.Args()) {
	case 0:
		if !com.IsFile(".gopmfile") {
			break
		}

		gf := doc.NewGopmfile(".")
		target = gf.MustValue("target", "path")
	case 1:
		target = ctx.Args()[0]
	default:
		log.Fatal("install", "Too many arguments")
	}

	if !ctx.Bool("remote") {
		// Get GOPATH.
		installGopath = com.GetGOPATHs()[0]
		if !com.IsDir(installGopath) {
			log.Error("install", "Invalid GOPATH path")
			log.Error("", "GOPATH does not exist or is not a directory:")
			log.Error("", "\t"+installGopath)
			log.Help("Try 'go help gopath' to get more information")
		}
		log.Log("Indicated GOPATH: %s", installGopath)
		installGopath += "/src"
	}

	genNewGoPath(ctx, false)

	var installRepos []string
	if ctx.Bool("pkg") {
		curPath, _ := filepath.Abs(".")
		installRepos = doc.GetAllImports([]string{curPath},
			".", ctx.Bool("example"))
	} else {
		if len(target) == 0 {
			target = pkgName
		}

		installRepos = []string{target}
	}

	log.Trace("Installing...")

	for _, repo := range installRepos {
		cmdArgs := []string{"go", "install"}

		if ctx.Bool("verbose") {
			cmdArgs = append(cmdArgs, "-v")
		}
		cmdArgs = append(cmdArgs, repo)
		err := execCmd(newGoPath, newCurPath, cmdArgs...)
		if err != nil {
			log.Error("install", "Fail to install program:")
			log.Fatal("", "\t"+err.Error())
		}
	}

	log.Success("SUCC", "install", "Command executed successfully!")
}
