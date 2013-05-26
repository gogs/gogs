// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/GPMGo/gopm/doc"
	"github.com/GPMGo/gopm/utils"
	"github.com/GPMGo/node"
)

var CmdCheck = &Command{
	UsageLine: "check [check flags] [packages]",
}

func init() {
	CmdCheck.Run = runCheck
	CmdCheck.Flags = map[string]bool{
		"-e": false,
	}
}

// printCheckPrompt prints prompt information to users to
// let them know what's going on.
func printCheckPrompt(flag string) {
	switch flag {
	case "-e":
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["CheckExDeps"]))
	}
}

func runCheck(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, Config.AutoEnable.Check, args, printCheckPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	wd, _ := os.Getwd()
	// Guess import path.
	gopath := utils.GetBestMatchGOPATH(wd) + "/src/"
	if len(wd) <= len(gopath) {
		fmt.Printf(fmt.Sprintf("runCheck -> %s\n", PromptMsg["InvalidPath"]))
		return
	}

	importPath := wd[len(gopath):]
	imports, err := checkImportsByRoot(wd+"/", importPath)
	if err != nil {
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("runCheck -> %s\n", PromptMsg["CheckImports"]), err))
		return
	}

	if len(imports) == 0 {
		return
	}

	importsCache := make(map[string]bool)
	uninstallList := make([]string, 0)
	isInstalled := false
	// Check if dependencies have been installed.
	paths := utils.GetGOPATH()

	for _, v := range imports {
		// Make sure it doesn't belong to same project.
		if utils.GetProjectPath(v) != utils.GetProjectPath(importPath) {
			for _, p := range paths {
				if checkIsExistWithVCS(p + "/src/" + v + "/") {
					isInstalled = true
					break
				}
			}

			if !isInstalled && !importsCache[v] {
				importsCache[v] = true
				uninstallList = append(uninstallList, v)
			}
		}
		isInstalled = false
	}

	// Check if need to install packages.
	if len(uninstallList) > 0 {
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["MissingImports"]))
		for _, v := range uninstallList {
			fmt.Printf("%s\n", v)
		}
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["ContinueDownload"]))
		var option string
		fmt.Fscan(os.Stdin, &option)
		if strings.ToLower(option) != "y" {
			os.Exit(0)
		}

		installGOPATH = utils.GetBestMatchGOPATH(AppPath)
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("%s\n", PromptMsg["DownloadPath"]), installGOPATH))
		// Generate temporary nodes.
		nodes := make([]*node.Node, len(uninstallList))
		for i := range nodes {
			nodes[i] = new(node.Node)
			nodes[i].ImportPath = uninstallList[i]
		}
		// Download packages.
		downloadPackages(nodes)

		removePackageFiles("", uninstallList)

		// Install packages all together.
		var cmdArgs []string
		cmdArgs = append(cmdArgs, "install")
		cmdArgs = append(cmdArgs, "<blank>")

		for _, k := range uninstallList {
			fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["InstallStatus"]), k)
			cmdArgs[1] = k
			executeCommand("go", cmdArgs)
		}

		// Generate configure file.
	}
}

// checkImportsByRoot checks imports of packages from root path,
// and recursion checks all sub-directories.
func checkImportsByRoot(rootPath, importPath string) (imports []string, err error) {
	// Check imports of root path.
	importPkgs, err := doc.CheckImports(rootPath, importPath)
	if err != nil {
		return nil, err
	}
	imports = append(imports, importPkgs...)

	// Check sub-directories.
	dirs, err := utils.GetDirsInfo(rootPath)
	if err != nil {
		return nil, err
	}

	for _, d := range dirs {
		if d.IsDir() &&
			!(!CmdCheck.Flags["-e"] && strings.Contains(d.Name(), "example")) {
			importPkgs, err := checkImportsByRoot(rootPath+d.Name()+"/", importPath)
			if err != nil {
				return nil, err
			}
			imports = append(imports, importPkgs...)
		}
	}

	return imports, err
}

// checkIsExistWithVCS returns false if directory only has VCS folder,
// or doesn't exist.
func checkIsExistWithVCS(path string) bool {
	// Check if directory exist.
	if !utils.IsExist(path) {
		return false
	}

	// Check if only has VCS folder.
	dirs, err := utils.GetDirsInfo(path)
	if err != nil {
		utils.ColorPrint(fmt.Sprintf("[ERROR] checkIsExistWithVCS -> [ %s ]", err))
		return false
	}

	if len(dirs) > 1 {
		return true
	} else if len(dirs) == 0 {
		return false
	}

	switch dirs[0].Name() {
	case ".git", ".hg", ".svn":
		return false
	}

	return true
}
