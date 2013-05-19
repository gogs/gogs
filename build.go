// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/GPMGo/gpm/utils"
)

var cmdBuild = &Command{
	UsageLine: "build [build flags] [packages]",
}

func init() {
	cmdBuild.Run = runBuild
}

func runBuild(cmd *Command, args []string) {
	var cmdArgs []string
	cmdArgs = append(cmdArgs, "install")
	cmdArgs = append(cmdArgs, args...)

	wd, _ := os.Getwd()
	wd = strings.Replace(wd, "\\", "/", -1)
	proName := path.Base(wd)
	if runtime.GOOS == "windows" {
		proName += ".exe"
	}

	executeGoCommand(cmdArgs)

	// Find executable in GOPATH and copy to current directory.
	paths := utils.GetGOPATH()

	for _, v := range paths {
		if utils.IsExist(v + "/bin/" + proName) {
			err := os.Remove(wd + "/" + proName)
			if err != nil {
				fmt.Printf("Fail to remove file in current directory: %s.\n", err)
				return
			}
			err = os.Rename(v+"/bin/"+proName, wd+"/"+proName)
			if err == nil {
				fmt.Printf("Moved file from $GOPATH(%s) to current directory(%s).\n", v, wd)
				return
			} else {
				fmt.Printf("Fail to move file from $GOPATH(%s) to current directory: %s.\n", v, err)
			}
			break
		}
	}
}
