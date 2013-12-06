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

	"github.com/gpmgo/gopm/log"
)

var CmdRun = cli.Command{
	Name:  "run",
	Usage: "link dependencies and go run",
	Description: `Command run links dependencies according to gopmfile,
and execute 'go run'

gopm run <go run commands>`,
	Action: runRun,
}

func runRun(ctx *cli.Context) {
	setup(ctx)

	// Get GOPATH.
	installGopath = com.GetGOPATHs()[0]
	if com.IsDir(installGopath) {
		isHasGopath = true
		log.Log("Indicated GOPATH: %s", installGopath)
		installGopath += "/src"
	}

	genNewGoPath(ctx, false)

	log.Trace("Running...")

	cmdArgs := []string{"go", "run"}
	cmdArgs = append(cmdArgs, ctx.Args()...)
	err := execCmd(newGoPath, newCurPath, cmdArgs...)
	if err != nil {
		log.Error("run", "Fail to run program:")
		log.Fatal("", "\t"+err.Error())
	}

	log.Success("SUCC", "run", "Command executed successfully!")
}
