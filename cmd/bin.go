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
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdBin = cli.Command{
	Name:  "bin",
	Usage: "download and link dependencies and build executable binary",
	Description: `Command bin downloads and links dependencies according to gopmfile,
and build executable binary to work directory

gopm bin <import path>@[<tag|commit|branch>:<value>]
gopm bin <package name>@[<tag|commit|branch>:<value>]

Can only specify one each time, and only works for projects that 
contains main package`,
	Action: runBin,
	Flags: []cli.Flag{
		cli.BoolFlag{"dir, d", "build binary to given directory(second argument)"},
	},
}

func runBin(ctx *cli.Context) {
	if len(ctx.Args()) == 0 {
		log.Error("Bin", "Fail to start command")
		log.Fatal("", "No package specified")
	}

	doc.LoadPkgNameList(doc.HomeDir + "/data/pkgname.list")

	installRepoPath = doc.HomeDir + "/repos"

	// Check arguments.
	num := 1
	if ctx.Bool("dir") {
		num = 2
	}
	if len(ctx.Args()) != num {
		log.Error("Bin", "Fail to start command")
		log.Fatal("", "Invalid argument number")
	}

	// Check if given directory exists.
	if ctx.Bool("dir") && !com.IsDir(ctx.Args()[1]) {
		log.Error("Bin", "Fail to start command")
		log.Fatal("", "Given directory does not exist")
	}

	// Parse package version.
	info := ctx.Args()[0]
	pkgPath := info
	ver := ""
	var err error
	if i := strings.Index(info, "@"); i > -1 {
		pkgPath = info[:i]
		_, ver, err = validPath(info[i+1:])
		if err != nil {
			log.Error("Bin", "Fail to parse version")
			log.Fatal("", err.Error())
		}
	}

	// Check package name.
	if !strings.Contains(pkgPath, "/") {
		name, ok := doc.PackageNameList[pkgPath]
		if !ok {
			log.Error("Bin", "Invalid package name: "+pkgPath)
			log.Fatal("", "No match in the package name list")
		}
		pkgPath = name
	}

	// Get code.
	stdout, _, _ := com.ExecCmd("gopm", "get", ctx.Args()[0])
	if len(stdout) > 0 {
		fmt.Print(stdout)
	}

	// Check if previous steps were successful.
	repoPath := installRepoPath + "/" + pkgPath
	if len(ver) > 0 {
		repoPath += "." + ver
	}
	if !com.IsDir(repoPath) {
		log.Error("Bin", "Fail to continue command")
		log.Fatal("", "Previous steps weren't successful")
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Error("Bin", "Fail to get work directory")
		log.Fatal("", err.Error())
	}

	// Change to repository path.
	log.Log("Changing work directory to %s", repoPath)
	err = os.Chdir(repoPath)
	if err != nil {
		log.Error("Bin", "Fail to change work directory")
		log.Fatal("", err.Error())
	}

	// Build application.
	stdout, _, _ = com.ExecCmd("gopm", "build")
	if len(stdout) > 0 {
		fmt.Print(stdout)
	}
	defer func() {
		// Clean files.
		os.RemoveAll(path.Join(repoPath, VENDOR))
	}()

	// Check if previous steps were successful.
	binName := path.Base(pkgPath)
	binPath := path.Join(VENDOR, "src", binName)
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath = path.Join(binPath, binName)
	if !com.IsFile(binPath) {
		log.Error("Bin", "Fail to continue command")
		log.Fatal("", "Previous steps weren't successful or the project does not contain main package")
	}

	// Move binary to given directory.
	movePath := wd
	if ctx.Bool("dir") {
		movePath = ctx.Args()[1]
	}
	err = os.Rename(binPath, movePath+"/"+binName)
	if err != nil {
		log.Error("Bin", "Fail to move binary")
		log.Fatal("", err.Error())
	}
	os.Chmod(movePath+"/"+binName, os.ModePerm)

	log.Log("Changing work directory back to %s", wd)
	os.Chdir(wd)

	log.Success("SUCC", "Bin", "Command execute successfully!")
}
