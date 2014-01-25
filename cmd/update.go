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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var CmdUpdate = cli.Command{
	Name:  "update",
	Usage: "check and update gopm resources including itself",
	Description: `Command update checks updates of resources and gopm itself.

gopm update

Resources will be updated automatically after executed this command,
but you have to confirm before updaing gopm itself.`,
	Action: runUpdate,
	Flags: []cli.Flag{
		cli.BoolFlag{"verbose, v", "show process details"},
	},
}

func exePath() string {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		log.Error("Update", "Fail to execute exec.LookPath")
		log.Fatal("", err.Error())
	}
	path, err := filepath.Abs(file)
	if err != nil {
		log.Error("Update", "Fail to get absolutely path")
		log.Fatal("", err.Error())
	}
	return path
}

type version struct {
	Gopm            int
	PackageNameList int `json:"package_name_list"`
}

func runUpdate(ctx *cli.Context) {
	setup(ctx)

	isAnythingUpdated := false
	// Load local version info.
	localVerInfo := loadLocalVerInfo()

	// Get remote version info.
	var remoteVerInfo version
	if err := com.HttpGetJSON(http.DefaultClient, "http://gopm.io/VERSION.json", &remoteVerInfo); err != nil {
		log.Error("Update", "Fail to fetch VERSION.json")
		log.Fatal("", err.Error())
	}

	// Package name list.
	if remoteVerInfo.PackageNameList > localVerInfo.PackageNameList {
		log.Log("Updating pkgname.list...%v > %v",
			localVerInfo.PackageNameList, remoteVerInfo.PackageNameList)
		data, err := com.HttpGetBytes(http.DefaultClient, "https://raw2.github.com/gpmgo/docs/master/pkgname.list", nil)
		if err != nil {
			log.Error("Update", "Fail to update pkgname.list")
			log.Fatal("", err.Error())
		}
		_, err = com.SaveFile(path.Join(doc.HomeDir, doc.PKG_NAME_LIST_PATH), data)
		if err != nil {
			log.Error("Update", "Fail to save pkgname.list")
			log.Fatal("", err.Error())
		}
		log.Log("Update pkgname.list to %v succeed!", remoteVerInfo.PackageNameList)
		isAnythingUpdated = true
	}

	// Gopm.
	if remoteVerInfo.Gopm > localVerInfo.Gopm {
		log.Log("Updating gopm...%v > %v",
			localVerInfo.Gopm, remoteVerInfo.Gopm)
		installRepoPath = doc.HomeDir + "/repos"

		tmpDirPath := filepath.Join(doc.HomeDir, "temp")
		tmpBinPath := filepath.Join(tmpDirPath, "gopm")
		if runtime.GOOS == "windows" {
			tmpBinPath += ".exe"
		}

		os.MkdirAll(tmpDirPath, os.ModePerm)
		os.Remove(tmpBinPath)

		// Fetch code.
		args := []string{"bin", "-u", "-d"}
		if ctx.Bool("verbose") {
			args = append(args, "-v")
		}
		args = append(args, []string{"github.com/gpmgo/gopm", tmpDirPath}...)
		stdout, stderr, err := com.ExecCmd("gopm", args...)
		if err != nil {
			log.Error("Update", "Fail to execute 'gopm bin -u -d github.com/gpmgo/gopm "+tmpDirPath+"'")
			log.Fatal("", err.Error())
		}
		if len(stderr) > 0 {
			fmt.Print(stderr)
		}
		if len(stdout) > 0 {
			fmt.Print(stdout)
		}

		// Check if previous steps were successful.
		if !com.IsExist(tmpBinPath) {
			log.Error("Update", "Fail to continue command")
			log.Fatal("", "Previous steps weren't successful, no binary produced")
		}

		movePath := exePath()
		log.Log("New binary will be replaced for %s", movePath)
		// Move binary to given directory.
		if runtime.GOOS != "windows" {
			err := os.Rename(tmpBinPath, movePath)
			if err != nil {
				log.Error("Update", "Fail to move binary")
				log.Fatal("", err.Error())
			}
			os.Chmod(movePath+"/"+path.Base(tmpBinPath), os.ModePerm)
		} else {
			batPath := filepath.Join(tmpDirPath, "update.bat")
			f, err := os.Create(batPath)
			if err != nil {
				log.Error("Update", "Fail to generate bat file")
				log.Fatal("", err.Error())
			}
			f.WriteString("@echo off\r\n")
			f.WriteString(fmt.Sprintf("ping -n 1 127.0.0.1>nul\r\ncopy \"%v\" \"%v\" >nul\r\ndel \"%v\" >nul\r\n\r\n",
				tmpBinPath, movePath, tmpBinPath))
			//f.WriteString(fmt.Sprintf("del \"%v\"\r\n", batPath))
			f.Close()

			attr := &os.ProcAttr{
				Dir:   workDir,
				Env:   os.Environ(),
				Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			}

			_, err = os.StartProcess(batPath, []string{batPath}, attr)
			if err != nil {
				log.Error("Update", "Fail to start bat process")
				log.Fatal("", err.Error())
			}
		}

		log.Success("SUCC", "Update", "Command execute successfully!")
		isAnythingUpdated = true
	}

	// Save JSON.
	f, err := os.Create(path.Join(doc.HomeDir, doc.VER_PATH))
	if err != nil {
		log.Error("Update", "Fail to create VERSION.json")
		log.Fatal("", err.Error())
	}
	if err := json.NewEncoder(f).Encode(&remoteVerInfo); err != nil {
		log.Error("Update", "Fail to encode VERSION.json")
		log.Fatal("", err.Error())
	}

	if !isAnythingUpdated {
		log.Log("Nothing need to be updated")
	}
	log.Log("Exit old gopm")
}

func loadLocalVerInfo() (ver version) {
	verPath := path.Join(doc.HomeDir, doc.VER_PATH)

	// First time run should not exist.
	if !com.IsExist(verPath) {
		return ver
	}

	f, err := os.Open(verPath)
	if err != nil {
		log.Error("Update", "Fail to open VERSION.json")
		log.Fatal("", err.Error())
	}

	if err := json.NewDecoder(f).Decode(&ver); err != nil {
		log.Error("Update", "Fail to decode VERSION.json")
		log.Fatal("", err.Error())
	}
	return ver
}
