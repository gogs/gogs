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

	"github.com/Unknwon/com"
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
	Flags: []cli.Flag{
		cli.BoolFlag{"update, u", "update pakcage(s) and dependencies if any"},
		cli.BoolFlag{"verbose, v", "show process details"},
	},
}

func runBuild(ctx *cli.Context) {
	setup(ctx)

	// Get GOPATH.
	installGopath = com.GetGOPATHs()[0]
	if com.IsDir(installGopath) {
		isHasGopath = true
		log.Log("Indicated GOPATH: %s", installGopath)
		installGopath += "/src"
	}

	buildBinary(ctx, ctx.Args()...)
	log.Success("SUCC", "build", "Command executed successfully!")
}

func buildBinary(ctx *cli.Context, args ...string) {
	genNewGoPath(ctx, false)

	log.Trace("Building...")

	cmdArgs := []string{"go", "build"}
	cmdArgs = append(cmdArgs, args...)
	err := execCmd(newGoPath, newCurPath, cmdArgs...)
	if err != nil {
		log.Error("build", "fail to build program:")
		log.Fatal("", "\t"+err.Error())
	}

	if isWindowsXP {
		binName := pkgName + ".exe"
		os.Remove(binName)
		if com.IsFile(path.Join(doc.VENDOR, "src", pkgName, binName)) {
			err = os.Rename(path.Join(doc.VENDOR, "src", pkgName, binName), binName)
			if err != nil {
				log.Error("build", "fail to move binary:")
				log.Fatal("", "\t"+err.Error())
			}
		} else {
			log.Warn("No binary generated")
		}
	}
}
