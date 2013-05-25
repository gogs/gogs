// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/GPMGo/gopm/utils"
)

var cmdBuild = &Command{
	UsageLine: "build [build flags] [packages]",
}

func init() {
	cmdBuild.Run = runBuild
	cmdBuild.Flags = map[string]bool{
		"-v": false,
		"-r": false,
	}
}

// printBuildPrompt prints prompt information to users to
// let them know what's going on.
func printBuildPrompt(flag string) {
	switch flag {

	}
}

func runBuild(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, config.AutoEnable.Build, args, printBuildPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	var cmdArgs []string
	cmdArgs = append(cmdArgs, "install")
	if cmdBuild.Flags["-v"] {
		cmdArgs = append(cmdArgs, "-v")
	}

	executeCommand("go", cmdArgs)

	// Find executable in GOPATH and copy to current directory.
	wd, _ := os.Getwd()
	proName := utils.GetExecuteName(wd)
	paths := utils.GetGOPATH()

	for _, v := range paths {
		if utils.IsExist(v + "/bin/" + proName) {
			if utils.IsExist(wd + "/" + proName) {
				err := os.Remove(wd + "/" + proName)
				if err != nil {
					fmt.Printf(fmt.Sprintf("ERROR: %s\n", promptMsg["RemoveFile"]), err)
					return
				}
			}
			err := os.Rename(v+"/bin/"+proName, wd+"/"+proName)
			if err == nil {
				fmt.Printf(fmt.Sprintf("%s\n", promptMsg["MovedFile"]), v, wd)
				// Check if need to run program.
				if cmdBuild.Flags["-r"] {
					cmdArgs = make([]string, 0)
					executeCommand(proName, cmdArgs)
				}
				return
			}

			fmt.Printf(fmt.Sprintf("%s\n", promptMsg["MoveFile"]), v, wd)
			break
		}
	}
}
