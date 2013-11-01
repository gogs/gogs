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
	"go/build"
	"os"
	"os/exec"
)

var CmdRun = &Command{
	UsageLine: "run",
	Short:     "run according a gopmfile",
	Long: `
run just like go run
`,
}

func init() {
	CmdRun.Run = runRun
	CmdRun.Flags = map[string]bool{}
}

func printRunPrompt(flag string) {
}

func runRun(cmd *Command, args []string) {
	gopath := build.Default.GOPATH

	genNewGoPath()

	com.ColorLog("[INFO] running ...\n")

	cmdArgs := []string{"go", "run"}
	cmdArgs = append(cmdArgs, args...)
	bCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	bCmd.Stdout = os.Stdout
	bCmd.Stderr = os.Stderr
	err := bCmd.Run()
	if err != nil {
		com.ColorLog("[ERRO] run failed: %v\n", err)
		return
	}

	com.ColorLog("[TRAC] set GOPATH=%v\n", gopath)
	err = os.Setenv("GOPATH", gopath)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	com.ColorLog("[SUCC] run successfully!\n")
}
