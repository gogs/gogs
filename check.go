// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/GPMGo/gpm/doc"
	"github.com/GPMGo/gpm/utils"
)

var cmdCheck = &Command{
	UsageLine: "check [check flags] [packages]",
}

func init() {
	cmdCheck.Run = runCheck
}

// printCheckPrompt prints prompt information to users to
// let them know what's going on.
func printCheckPrompt(flag string) {
	switch flag {

	}
}

func runCheck(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, config.AutoEnable.Check, args, printCheckPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	wd, _ := os.Getwd()
	// Guess import path.
	gopath := utils.GetBestMatchGOPATH(wd) + "/src/"
	if len(wd) <= len(gopath) {
		fmt.Printf(fmt.Sprintf("%s\n", promptMsg["InvalidPath"]))
		return
	}

	importPath := wd[len(gopath):]
	imports, err := doc.CheckImports(wd+"/", importPath)
	if err != nil {
		fmt.Printf(fmt.Sprintf("%s\n", promptMsg["CheckImports"]), err)
		return
	}

	if len(imports) == 0 {
		return
	}

	uninstallList := make([]string, 0)
	isInstalled := false
	// Check if dependencies have been installed.
	paths := utils.GetGOPATH()

	for _, v := range imports {
		// Make sure it doesn't belong to same project.
		if utils.GetProjectPath(v) != utils.GetProjectPath(importPath) {
			for _, p := range paths {
				if utils.IsExist(p + "/src/" + v + "/") {
					isInstalled = true
					break
				}
			}

			if !isInstalled {
				uninstallList = append(uninstallList, v)
			}
		}
	}

	// Check if need to install packages.
	if len(uninstallList) > 0 {
		fmt.Printf(fmt.Sprintf("%s\n", promptMsg["MissingImports"]))
		for _, v := range uninstallList {
			fmt.Printf("%s\n", v)
		}
		fmt.Printf(fmt.Sprintf("%s\n", promptMsg["ContinueDownload"]))
		var option string
		fmt.Fscan(os.Stdin, &option)
		if strings.ToLower(option) != "y" {
			os.Exit(0)
		}

		installGOPATH = utils.GetBestMatchGOPATH(appPath)
		fmt.Printf(fmt.Sprintf("%s\n", promptMsg["DownloadPath"]), installGOPATH)
		// Generate temporary nodes.
		nodes := make([]*doc.Node, len(uninstallList))
		for i := range nodes {
			nodes[i] = new(doc.Node)
			nodes[i].ImportPath = uninstallList[i]
		}
		// Download packages.
		downloadPackages(nodes)

		// Install packages all together.
		var cmdArgs []string
		cmdArgs = append(cmdArgs, "install")
		cmdArgs = append(cmdArgs, "<blank>")

		paths := utils.GetGOPATH()
		pkgPath := "/pkg/" + runtime.GOOS + "_" + runtime.GOARCH + "/"
		for _, k := range uninstallList {
			// Delete old packages.
			for _, p := range paths {
				os.RemoveAll(p + pkgPath + k + "/")
				os.Remove(p + pkgPath + k + ".a")
			}
		}

		for _, k := range uninstallList {
			fmt.Printf(fmt.Sprintf("%s\n", promptMsg["InstallStatus"]), k)
			cmdArgs[1] = k
			executeCommand("go", cmdArgs)
		}

		// Generate configure file.
	}
}
