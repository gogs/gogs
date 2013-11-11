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
	"fmt"
	"go/build"
	"os"
	"os/exec"

	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/log"
)

var CmdRun = cli.Command{
	Name:  "run",
	Usage: "link dependencies and go run",
	Description: `Command run links dependencies according to gopmfile

gopm run <file names>`,
	Action: runRun,
}

func runRun(ctx *cli.Context) {
	gopath := build.Default.GOPATH

	genNewGoPath(ctx)

	cmdArgs := []string{"go", "run"}
	cmdArgs = append(cmdArgs, ctx.Args()...)
	bCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	bCmd.Stdout = os.Stdout
	bCmd.Stderr = os.Stderr

	log.Log("===== application outputs start =====\n")

	err := bCmd.Run()

	fmt.Println()
	log.Log("====== application outputs end ======")

	if err != nil {
		log.Error("Run", "Fail to execute")
		log.Fatal("", err.Error())
	}

	log.Trace("Set back GOPATH=%s", gopath)
	err = os.Setenv("GOPATH", gopath)
	if err != nil {
		log.Error("Run", "Fail to set back GOPATH")
		log.Fatal("", err.Error())
	}

	log.Success("SUCC", "Run", "Command execute successfully!")
}
