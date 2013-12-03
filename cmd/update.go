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
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdUpdate = cli.Command{
	Name:  "update",
	Usage: "update self",
	Description: `Command bin downloads and links dependencies according to gopmfile,
and build executable binary to work directory

gopm update

Can only specify one each time, and only works for projects that 
contains main package`,
	Action: runUpdate,
}

func exePath() string {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	return path
}

func runUpdate(ctx *cli.Context) {
	doc.LoadPkgNameList(doc.HomeDir + "/data/pkgname.list")

	installRepoPath = doc.HomeDir + "/repos"

	// Check arguments.
	num := 0

	if len(ctx.Args()) != num {
		log.Error("Update", "Fail to start command")
		log.Fatal("", "Invalid argument number")
	}

	// Parse package version.
	info := "github.com/gpmgo/gopm"
	pkgPath := info
	ver := ""
	var err error
	if i := strings.Index(info, "@"); i > -1 {
		pkgPath = info[:i]
		_, ver, err = validPath(info[i+1:])
		if err != nil {
			log.Error("Update", "Fail to parse version")
			log.Fatal("", err.Error())
		}
	}

	// Check package name.
	if !strings.Contains(pkgPath, "/") {
		name, ok := doc.PackageNameList[pkgPath]
		if !ok {
			log.Error("Update", "Invalid package name: "+pkgPath)
			log.Fatal("", "No match in the package name list")
		}
		pkgPath = name
	}

	// Get code.
	stdout, _, _ := com.ExecCmd("gopm", "get", info)
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
	if com.IsFile(doc.GopmFileName) {
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

	binName := path.Base(pkgName)
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := path.Join(VENDOR, "src", pkgPath, binName)
	if !com.IsFile(binPath) {
		log.Error("Update", "Fail to continue command")
		log.Fatal("", "Previous steps weren't successful or the project does not contain main package")
	}

	movePath := exePath()
	fmt.Print(movePath)

	// Move binary to given directory.
	if runtime.GOOS != "windows" {
		err = os.Rename(binPath, movePath)
		if err != nil {
			log.Error("Update", "Fail to move binary")
			log.Fatal("", err.Error())
		}
		os.Chmod(movePath+"/"+binName, os.ModePerm)
	} else {
		batPath := filepath.Join(wd, "a.bat")
		f, err := os.Create(batPath)
		if err != nil {
			log.Error("Update", "Fail to generate bat file")
			log.Fatal("", err.Error())
		}
		f.WriteString(fmt.Sprintf(`ping -n 1 127.0.0.1>nul\ncopy "%v" "%v"\ndel "%v"`,
			binPath, movePath, binPath))
		f.Close()

		attr := &os.ProcAttr{
			Dir: wd,
			Env: os.Environ(),
			//Files: []*os.File{nil, nil, nil},
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		}

		_, err = os.StartProcess(batPath, []string{"a.bat"}, attr)
		if err != nil {
			log.Error("Update", "Fail to start bat process")
			log.Fatal("", err.Error())
		}
	}

	log.Log("Changing work directory back to %s", wd)
	os.Chdir(wd)

	log.Success("SUCC", "Update", "Command execute successfully!")
}
