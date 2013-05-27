// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
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

	pkgConf := new(gopmConfig)
	importsCache := make(map[string]bool)
	uninstallList := make([]string, 0)
	isInstalled := false
	// Check if dependencies have been installed.

	for _, v := range imports {
		// Make sure it doesn't belong to same project.
		if utils.GetProjectPath(v) != utils.GetProjectPath(importPath) {
			if !importsCache[v] {
				importsCache[v] = true
				pkgConf.Deps = append(pkgConf.Deps, &node.Node{
					ImportPath: v,
				})

				if _, ok := utils.CheckIsExistInGOPATH(importPath); ok {
					isInstalled = true
				}

				if !isInstalled {
					uninstallList = append(uninstallList, v)
				}
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
	}

	// Generate configure file.
	if !utils.IsExist("gopm.json") {
		fw, err := os.Create("gopm.json")
		if err != nil {
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] runCheck -> %s\n", PromptMsg["OpenFile"]), err))
			return
		}
		defer fw.Close()

		fbytes, err := json.MarshalIndent(&pkgConf, "", "\t")
		if err != nil {
			utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] runCheck -> %s\n", PromptMsg["ParseJSON"]), err))
			return
		}
		fw.Write(fbytes)
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("<SUCCESS>$ %s\n", PromptMsg["GenerateConfig"]), importPath))
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
