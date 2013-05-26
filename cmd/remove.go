// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/GPMGo/gopm/doc"
	"github.com/GPMGo/gopm/utils"
)

var (
	removeCache map[string]bool // Saves packages that have been removed.
)

var CmdRemove = &Command{
	UsageLine: "remove [remove flags] <packages|bundles|snapshots>",
}

func init() {
	removeCache = make(map[string]bool)
	CmdRemove.Run = runRemove
}

func runRemove(cmd *Command, args []string) {
	// Check length of arguments.
	if len(args) < 1 {
		fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["NoPackage"]))
		return
	}

	// Generate temporary nodes.
	nodes := make([]*doc.Node, len(args))
	for i := range nodes {
		nodes[i] = new(doc.Node)
		nodes[i].ImportPath = args[i]
	}

	// Removes packages.
	removePackages(nodes)

	// Save local nodes to file.
	fw, err := os.Create(AppPath + "data/nodes.json")
	if err != nil {
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] runRemove -> %s\n", PromptMsg["OpenFile"]), err))
		return
	}
	defer fw.Close()
	fbytes, err := json.MarshalIndent(&LocalNodes, "", "\t")
	if err != nil {
		utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("[ERROR] runRemove -> %s\n", PromptMsg["ParseJSON"]), err))
		return
	}
	fw.Write(fbytes)
}

// removePackages removes packages from local file system.
func removePackages(nodes []*doc.Node) {
	// Check all packages, they may be bundles, snapshots or raw packages path.
	for _, n := range nodes {
		// Check if it is a bundle or snapshot.
		switch {
		case strings.HasSuffix(n.ImportPath, ".b"):
			l := len(n.ImportPath)
			// Check local bundles.
			bnodes := checkLocalBundles(n.ImportPath[:l-2])
			if len(bnodes) > 0 {
				// Check with users if continue.
				utils.ColorPrint(fmt.Sprintf(fmt.Sprintf("%s\n", PromptMsg["BundleInfo"]), n.ImportPath[:l-2]))
				for _, bn := range bnodes {
					fmt.Printf("[%s] -> %s: %s.\n", bn.ImportPath, bn.Type, bn.Value)
				}
				fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["ContinueRemove"]))
				var option string
				fmt.Fscan(os.Stdin, &option)
				if strings.ToLower(option) != "y" {
					os.Exit(0)
				}
				removePackages(bnodes)
			} else {
				// Check from server.
				// TODO: api.GetBundleInfo()
				fmt.Println("Unable to find bundle, and we cannot check with server right now.")
			}
		case strings.HasSuffix(n.ImportPath, ".s"):
		case utils.IsValidRemotePath(n.ImportPath):
			if !removeCache[n.ImportPath] {
				// Remove package.
				node, imports := removePackage(n)
				if len(imports) > 0 {
					fmt.Println("Check denpendencies for removing package has not been supported.")
				}

				// Remove record in local nodes.
				if node != nil {
					removeNode(node)
				}
			}
		default:
			// Invalid import path.
			fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["SkipInvalidPath"]), n.ImportPath)
		}
	}
}

// removeNode removes node from local nodes.
func removeNode(n *doc.Node) {
	// Check if this node exists.
	for i, v := range LocalNodes {
		if n.ImportPath == v.ImportPath {
			LocalNodes = append(LocalNodes[:i], LocalNodes[i+1:]...)
			return
		}
	}
}

// removePackage removes package from local file system.
func removePackage(node *doc.Node) (*doc.Node, []string) {
	// Find package in GOPATH.
	paths := utils.GetGOPATH()
	for _, p := range paths {
		absPath := p + "/src/" + utils.GetProjectPath(node.ImportPath) + "/"
		if utils.IsExist(absPath) {
			fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["RemovePackage"]), node.ImportPath)
			// Remove files.
			os.RemoveAll(absPath)
			// Remove file in GOPATH/bin
			proName := utils.GetExecuteName(node.ImportPath)
			paths := utils.GetGOPATH()
			var gopath string

			for _, v := range paths {
				if utils.IsExist(v + "/bin/" + proName) {
					gopath = v // Don't need to find again.
					os.Remove(v + "/bin/" + proName)
				}
			}

			pkgList := []string{node.ImportPath}
			removePackageFiles(gopath, pkgList)

			return node, nil
		}
	}

	// Cannot find package.
	fmt.Printf(fmt.Sprintf("%s\n", PromptMsg["PackageNotFound"]), node.ImportPath)
	return nil, nil
}

// removePackageFiles removes package files in $GOPATH/pkg.
func removePackageFiles(gopath string, pkgList []string) {
	var paths []string
	// Check if need to find GOPATH.
	if len(gopath) == 0 {
		paths = utils.GetGOPATH()
	} else {
		paths = append(paths, gopath)
	}

	pkgPath := "/pkg/" + runtime.GOOS + "_" + runtime.GOARCH + "/"
	for _, p := range pkgList {
		for _, g := range paths {
			os.RemoveAll(g + pkgPath + p + "/")
			os.Remove(g + pkgPath + p + ".a")
		}
	}
}
