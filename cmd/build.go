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
)

var CmdBuild = &Command{
	UsageLine: "build",
	Short:     "build according a gopmfile",
	Long: `
build just like go build
`,
}

func init() {
	CmdBuild.Run = runBuild
	CmdBuild.Flags = map[string]bool{}
}

func printBuildPrompt(flag string) {
}

func runBuild(cmd *Command, args []string) {
	//genNewGoPath()

	com.ColorLog("[INFO] building ...\n")

	cmds := []string{"go", "build"}
	cmds = append(cmds, args...)
	err := execCmd(newGoPath, newCurPath, cmds...)
	if err != nil {
		com.ColorLog("[ERRO] build failed: %v\n", err)
		return
	}

	com.ColorLog("[SUCC] build successfully!\n")
}
