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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var (
	installRepoPath string
	downloadCache   map[string]bool // Saves packages that have been downloaded.
	downloadCount   int
	failConut       int
)

var CmdGet = cli.Command{
	Name:  "get",
	Usage: "fetch remote package(s) and dependencies to local repository",
	Description: `Command get fetches a package, and any pakcages that it depents on. 
If the package has a gopmfile, the fetch process will be driven by that.

gopm get
gopm get <import path>@[<tag|commit|branch>:<value>]
gopm get <package name>@[<tag|commit|branch>:<value>]

Can specify one or more: gopm get beego@tag:v0.9.0 github.com/beego/bee

If no argument is supplied, then gopmfile must be present`,
	Action: runGet,
	Flags: []cli.Flag{
		cli.BoolFlag{"force", "force to update pakcage(s) and dependencies"},
		cli.BoolFlag{"example", "download dependencies for example(s)"},
	},
}

func init() {
	downloadCache = make(map[string]bool)
}

func runGet(ctx *cli.Context) {
	// Check number of arguments.
	switch len(ctx.Args()) {
	case 0:
		getByGopmfile(ctx)
	}
}

func getByGopmfile(ctx *cli.Context) {
	if !com.IsFile(".gopmfile") {
		log.Fatal("install", "No argument is supplied and no gopmfile exist")
	}

	hd, err := com.HomeDir()
	if err != nil {
		log.Error("install", "Fail to get current user")
		log.Fatal("", err.Error())
	}

	installRepoPath = strings.Replace(reposDir, "~", hd, -1)
	log.Log("Local repository path: %s", installRepoPath)

	// TODO: 获取依赖包

	log.Error("install", "command haven't done yet!")
}

func processGet() {

}

func runGet1(cmd *Command, args []string) {
	nodes := []*doc.Node{}
	// ver describles branch, tag or commit.
	var t, ver string = doc.BRANCH, ""

	var err error
	if len(args) >= 2 {
		t, ver, err = validPath(args[1])
		if err != nil {
			com.ColorLog("[ERROR] Fail to parse 'args'[ %s ]\n", err)
			return
		}
	}

	node := doc.NewNode(args[0], args[0], t, ver, true)
	nodes = append(nodes, node)

	// Download package(s).
	downloadPackages(nodes)

	com.ColorLog("[INFO] %d package(s) downloaded, %d failed.\n",
		downloadCount, failConut)
}

// printGetPrompt prints prompt information to users to
// let them know what's going on.
func printGetPrompt(flag string) {
	switch flag {
	case "-d":
		com.ColorLog("[INFO] You enabled download without installing.\n")
	case "-u":
		com.ColorLog("[INFO] You enabled force update.\n")
	case "-e":
		com.ColorLog("[INFO] You enabled download dependencies of example(s).\n")
	}
}

// checkFlags checks if the flag exists with correct format.
func checkFlags(flags map[string]bool, args []string, print func(string)) int {
	num := 0 // Number of valid flags, use to cut out.
	for i, f := range args {
		// Check flag prefix '-'.
		if !strings.HasPrefix(f, "-") {
			// Not a flag, finish check process.
			break
		}

		// Check if it a valid flag.
		if v, ok := flags[f]; ok {
			flags[f] = !v
			if !v {
				print(f)
			} else {
				fmt.Println("DISABLE: " + f)
			}
		} else {
			com.ColorLog("[ERRO] Unknown flag: %s.\n", f)
			return -1
		}
		num = i + 1
	}

	return num
}

// downloadPackages downloads packages with certain commit,
// if the commit is empty string, then it downloads all dependencies,
// otherwise, it only downloada package with specific commit only.
func downloadPackages(nodes []*doc.Node) {
	// Check all packages, they may be raw packages path.
	for _, n := range nodes {
		// Check if it is a valid remote path.
		if doc.IsValidRemotePath(n.ImportPath) {
			// if !CmdGet.Flags["-u"] {
			// 	// Check if package has been downloaded.
			// 	installPath := installRepoPath + "/" + doc.GetProjectPath(n.ImportPath)
			// 	if len(n.Value) > 0 {
			// 		installPath += "." + n.Value
			// 	}
			// 	if com.IsExist(installPath) {
			// 		com.ColorLog("[WARN] Skipped installed package( %s => %s:%s )\n",
			// 			n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))
			// 		continue
			// 	}
			// }

			if !downloadCache[n.ImportPath] {
				// Download package.
				nod, imports := downloadPackage(n)
				if len(imports) > 0 {
					// Need to download dependencies.
					// Generate temporary nodes.
					nodes := make([]*doc.Node, len(imports))
					for i := range nodes {
						nodes[i] = doc.NewNode(imports[i], imports[i], doc.BRANCH, "", true)
					}
					downloadPackages(nodes)
				}

				// Only save package information with specific commit.
				if nod != nil {
					// Save record in local nodes.
					com.ColorLog("[SUCC] Downloaded package( %s => %s:%s )\n",
						n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))
					downloadCount++
					//saveNode(nod)
				}
			} else {
				com.ColorLog("[WARN] Skipped downloaded package( %s => %s:%s )\n",
					n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))
			}
		} else if n.ImportPath == "C" {
			continue
		} else {
			// Invalid import path.
			com.ColorLog("[WARN] Skipped invalid package path( %s => %s:%s )\n",
				n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))
			failConut++
		}
	}
}

// downloadPackage downloads package either use version control tools or not.
func downloadPackage(nod *doc.Node) (*doc.Node, []string) {
	com.ColorLog("[TRAC] Downloading package( %s => %s:%s )\n",
		nod.ImportPath, nod.Type, doc.CheckNodeValue(nod.Value))
	// Mark as donwloaded.
	downloadCache[nod.ImportPath] = true

	imports, err := doc.PureDownload(nod, installRepoPath, nil) //CmdGet.Flags)

	if err != nil {
		com.ColorLog("[ERRO] Download falied( %s )[ %s ]\n", nod.ImportPath, err)
		failConut++
		os.RemoveAll(installRepoPath + "/" + doc.GetProjectPath(nod.ImportPath) + "/")
		return nil, nil
	}
	return nod, imports
}

// validPath checks if the information of the package is valid.
func validPath(info string) (string, string, error) {
	infos := strings.Split(info, ":")

	l := len(infos)
	switch {
	case l > 2:
		return "", "", errors.New("Invalid information of package")
	case l == 1:
		return doc.BRANCH, "", nil
	case l == 2:
		switch infos[1] {
		case doc.TRUNK, doc.MASTER, doc.DEFAULT:
			infos[1] = ""
		}
		return infos[0], infos[1], nil
	default:
		return "", "", errors.New("Cannot match any case")
	}
}
