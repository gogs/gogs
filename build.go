// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
	cmdBuild.Flags = []string{"-v"}
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

	cmdExec := exec.Command("go", cmdArgs...)
	stdout, err := cmdExec.StdoutPipe()
	if err != nil {
		fmt.Println(err)
	}
	stderr, err := cmdExec.StderrPipe()
	if err != nil {
		fmt.Println(err)
	}
	err = cmdExec.Start()
	if err != nil {
		fmt.Println(err)
	}
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	cmdExec.Wait()

	// Find executable in GOPATH and copy to current directory.
	gopath := strings.Replace(os.Getenv("GOPATH"), ";", ":", -1)
	gopath = strings.Replace(gopath, "\\", "/", -1)
	paths := strings.Split(gopath, ":")
	for _, v := range paths {
		if utils.IsExist(v + "/bin/" + proName) {
			err = os.Remove(wd + "/" + proName)
			if err != nil {
				fmt.Println("Fail to remove file in current directory :", err)
				return
			}
			err = os.Rename(v+"/bin/"+proName, wd+"/"+proName)
			if err == nil {
				fmt.Println("Moved file from $GOPATH to current directory.")
				return
			} else {
				fmt.Println("Fail to move file from $GOPATH to current directory :", err)
			}
			break
		}
	}
}
