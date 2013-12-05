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
	"path/filepath"
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
		cli.BoolFlag{"force, f", "force to update pakcage(s) and dependencies"},
	},
}

func runBin(ctx *cli.Context) {
	log.PureMode = ctx.GlobalBool("ide")

	if len(ctx.Args()) == 0 {
		log.Error("bin", "Cannot start command:")
		log.Fatal("", "\tNo package specified")
	}

	installRepoPath = doc.HomeDir + "/repos"

	// Check arguments.
	num := 1
	if ctx.Bool("dir") {
		num = 2
	}
	if len(ctx.Args()) != num {
		log.Error("bin", "Cannot start command:")
		log.Fatal("", "\tMissing indicated path to build binary")
	}

	// Check if given directory exists.
	if ctx.Bool("dir") && !com.IsDir(ctx.Args()[1]) {
		log.Error("bin", "Cannot start command:")
		log.Fatal("", "\tIndicated path does not exist or is not a directory")
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
			log.Error("bin", "Cannot parse package version")
			log.Error("", err.Error()+":")
			log.Error("", "\t"+info[i+1:])
			log.Help("Try 'gopm help get' to get more information")
		}
	}

	// Check package name.
	if !strings.Contains(pkgPath, "/") {
		pkgPath = doc.GetPkgFullPath(pkgPath)
	}

	// Get code.
	args := make([]string, 0, 4)
	if log.PureMode {
		args = append(args, "-ide")
	}
	args = append(args, []string{"get", "-r", ctx.Args()[0]}...)
	stdout, stderr, err := com.ExecCmd("gopm", args...)
	if err != nil {
		log.Error("bin", "Error occurs when 'gopm get -r':")
		log.Fatal("", "\r"+err.Error())
	}
	if len(stderr) > 0 {
		log.Error("bin", "Fail to 'gopm get -r':")
		log.Fatal("", "\r"+stderr)
	}
	if len(stdout) > 0 {
		fmt.Print(stdout)
	}

	// Check if previous steps were successful.
	repoPath := installRepoPath + "/" + pkgPath + versionSuffix(ver)
	if !com.IsDir(repoPath) {
		log.Error("bin", "Cannot continue command:")
		log.Fatal("", "\tPrevious steps weren't successful")
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Error("bin", "Cannot get work directory:")
		log.Fatal("", "\t"+err.Error())
	}

	// Change to repository path.
	log.Log("Changing work directory to %s", repoPath)
	err = os.Chdir(repoPath)
	if err != nil {
		log.Error("bin", "Fail to change work directory:")
		log.Fatal("", "\t"+err.Error())
	}

	// Build application.
	args = make([]string, 0, 2)
	if log.PureMode {
		args = append(args, "-ide")
	}
	args = append(args, "build")
	stdout, stderr, err = com.ExecCmd("gopm", args...)
	if err != nil {
		log.Error("bin", "Error occurs when 'gopm build':")
		log.Fatal("", "\r"+err.Error())
	}
	if len(stderr) > 0 {
		log.Error("bin", "Fail to 'gopm build':")
		log.Fatal("", "\r"+stderr)
	}
	if len(stdout) > 0 {
		fmt.Print(stdout)
	}
	defer func() {
		// Clean files.
		//os.RemoveAll(path.Join(repoPath, doc.VENDOR))
	}()

	// Check if previous steps were successful.
	if com.IsFile(doc.GOPM_FILE_NAME) {
		log.Trace("Loading gopmfile...")
		gf := doc.NewGopmfile(".")

		var err error
		pkgName, err = gf.GetValue("target", "path")
		if err == nil {
			log.Log("Target name: %s", pkgName)
		}
	}

	if len(pkgName) == 0 {
		_, pkgName = filepath.Split(pkgPath)
	}

	// Because build command moved binary to root path.
	binName := path.Base(pkgName)
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	if !com.IsFile(binName) {
		log.Error("bin", "Binary does not exist:")
		log.Error("", "\t"+binName)
		log.Fatal("", "\tPrevious steps weren't successful or the project does not contain main package")
	}

	// Move binary to given directory.
	movePath := wd
	if ctx.Bool("dir") {
		movePath = ctx.Args()[1]
	}

	if com.IsExist(movePath + "/" + binName) {
		err = os.Remove(movePath + "/" + binName)
		if err != nil {
			log.Warn("Cannot remove binary in work directory:")
			log.Warn("\t %s", err)
		}
	}

	err = os.Rename(binName, movePath+"/"+binName)
	if err != nil {
		log.Error("bin", "Fail to move binary:")
		log.Fatal("", "\t"+err.Error())
	}
	os.Chmod(movePath+"/"+binName, os.ModePerm)

	log.Log("Changing work directory back to %s", wd)
	os.Chdir(wd)

	log.Success("SUCC", "bin", "Command execute successfully!")
}
