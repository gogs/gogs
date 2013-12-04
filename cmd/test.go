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
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/log"
)

var CmdTest = cli.Command{
	Name:  "test",
	Usage: "link dependencies and go test",
	Description: `Command test links dependencies according to gopmfile,
and execute 'go test'

gopm test <go test commands>`,
	Action: runTest,
}

func runTest(ctx *cli.Context) {
	genNewGoPath(ctx, true)

	log.Trace("Testing...")

	cmdArgs := []string{"go", "test"}
	cmdArgs = append(cmdArgs, ctx.Args()...)
	err := execCmd(newGoPath, newCurPath, cmdArgs...)
	if err != nil {
		log.Error("Test", "Fail to test program")
		log.Fatal("", err.Error())
	}

	log.Success("SUCC", "Test", "Command execute successfully!")
}
