// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/GPMGo/gpm/doc"
	"github.com/GPMGo/gpm/utils"
)

var (
	removeCache map[string]bool // Saves packages that have been removed.
)

var cmdRemove = &Command{
	UsageLine: "remove [remove flags] <packages|bundles|snapshots>",
}

func init() {
	removeCache = make(map[string]bool)
	cmdRemove.Run = runRemove
}

func runRemove(cmd *Command, args []string) {
	// Check length of arguments.
	if len(args) < 1 {
		fmt.Printf(fmt.Sprintf("%s\n", promptMsg["NoPackage"]))
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
	fw, err := os.Create(appPath + "data/nodes.json")
	if err != nil {
		fmt.Printf(fmt.Sprintf("ERROR: runRemove -> %s\n", promptMsg["OpenFile"]), err)
		return
	}
	defer fw.Close()
	fbytes, err := json.MarshalIndent(&localNodes, "", "\t")
	if err != nil {
		fmt.Printf(fmt.Sprintf("ERROR: runRemove -> %s\n", promptMsg["ParseJSON"]), err)
		return
	}
	fw.Write(fbytes)

	fmt.Println("Well done.")
}

// removePackages removes packages from local file system.
func removePackages(nodes []*doc.Node) {
	// Check all packages, they may be bundles, snapshots or raw packages path.
	for _, n := range nodes {
		// Check if it is a bundle or snapshot.
		switch {
		case n.ImportPath[0] == 'B':
		case n.ImportPath[0] == 'S':
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
			fmt.Printf(fmt.Sprintf("%s\n", promptMsg["SkipInvalidPath"]), n.ImportPath)
		}
	}
}

// removeNode removes node from local nodes.
func removeNode(n *doc.Node) {
	// Check if this node exists.
	for i, v := range localNodes {
		if n.ImportPath == v.ImportPath {
			localNodes = append(localNodes[:i], localNodes[i+1:]...)
			return
		}
	}
}

// removePackage removes package from local file system.
func removePackage(node *doc.Node) (*doc.Node, []string) {
	// Find package in GOPATH.
	paths := utils.GetGOPATH()
	for _, p := range paths {
		absPath := p + "/src/" + node.ImportPath + "/"
		if utils.IsExist(absPath) {
			fmt.Printf(fmt.Sprintf("%s\n", promptMsg["RemovePackage"]), node.ImportPath)
			// Remove files.
			os.RemoveAll(absPath)
			return node, nil
		}
	}

	// Cannot find package.
	fmt.Printf(fmt.Sprintf("%s\n", promptMsg["PackageNotFound"]), node.ImportPath)
	return nil, nil
}
