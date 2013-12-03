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
	"os"
	"path"

	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdBuild = cli.Command{
	Name:  "build",
	Usage: "link dependencies and go build",
	Description: `Command build links dependencies according to gopmfile,
and execute 'go build'

gopm build <go build commands>`,
	Action: runBuild,
}

func runBuild(ctx *cli.Context) {
	doc.LoadLocalNodes()
	genNewGoPath(ctx, false)

	log.Trace("Building...")

	cmdArgs := []string{"go", "build"}
	cmdArgs = append(cmdArgs, ctx.Args()...)
	err := execCmd(newGoPath, newCurPath, cmdArgs...)
	if err != nil {
		log.Error("Build", "Fail to build program")
		log.Fatal("", err.Error())
	}

	if isWindowsXP {
		binName := pkgName + ".exe"
		os.Remove(binName)
		err = os.Rename(path.Join(VENDOR, "src", pkgName, binName), binName)
		if err != nil {
			log.Error("Build", "Fail to move binary")
			log.Fatal("", err.Error())
		}
	}

	log.Success("SUCC", "Build", "Command execute successfully!")
}
