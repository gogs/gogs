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
	"github.com/gpmgo/gopm/doc"
)

var (
	installRepoPath string
	downloadCache   map[string]bool // Saves packages that have been downloaded.
	downloadCount   int
	failConut       int
)

var CmdGet = &Command{
	UsageLine: "get [flags] <package(s)>",
	Short:     "download and install packages and dependencies",
	Long: `
Get downloads and installs the packages named by the import paths,
along with their dependencies.

This command works even you haven't installed any version control tool
such as git, hg, etc.

The install flags are:

	-d
		download without installing package(s).
	-u
		force to update pakcage(s).
	-e
		download dependencies for example(s).

The list flags accept a space-separated list of strings.

For more about specifying packages, see 'go help packages'.
`,
}

func init() {
	downloadCache = make(map[string]bool)
	CmdGet.Run = runGet
	CmdGet.Flags = map[string]bool{
		"-d": false,
		"-u": false,
		"-e": false,
	}
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

func runGet(cmd *Command, args []string) {
	// Check flags.
	num := checkFlags(cmd.Flags, args, printGetPrompt)
	if num == -1 {
		return
	}
	args = args[num:]

	// Check length of arguments.
	if len(args) < 1 {
		com.ColorLog("[ERRO] Please list the package that you want to install.\n")
		return
	}

	hd, err := com.HomeDir()
	if err != nil {
		com.ColorLog("[ERRO] Fail to get current user[ %s ]\n", err)
		return
	}

	installRepoPath = strings.Replace(reposDir, "~", hd, -1)
	com.ColorLog("[INFO] Packages will be installed into( %s )\n", installRepoPath)

	nodes := []*doc.Node{}
	// ver describles branch, tag or commit.
	var t, ver string = doc.BRANCH, ""

	if len(args) >= 2 {
		t, ver, err = validPath(args[1])
		if err != nil {
			com.ColorLog("[ERROR] Fail to parse 'args'[ %s ]\n", err)
			return
		}
	}

	nodes = append(nodes, &doc.Node{
		ImportPath:  args[0],
		DownloadURL: args[0],
		Type:        t,
		Value:       ver,
		IsGetDeps:   true,
	})

	// Download package(s).
	downloadPackages(nodes)

	com.ColorLog("[INFO] %d package(s) downloaded, %d failed.\n",
		downloadCount, failConut)
}

// downloadPackages downloads packages with certain commit,
// if the commit is empty string, then it downloads all dependencies,
// otherwise, it only downloada package with specific commit only.
func downloadPackages(nodes []*doc.Node) {
	// Check all packages, they may be raw packages path.
	for _, n := range nodes {
		// Check if it is a valid remote path.
		if doc.IsValidRemotePath(n.ImportPath) {
			if !CmdGet.Flags["-u"] {
				// Check if package has been downloaded.
				installPath := installRepoPath + "/" + doc.GetProjectPath(n.ImportPath)
				if len(n.Value) > 0 {
					installPath += "." + n.Value
				}
				if com.IsExist(installPath) {
					com.ColorLog("[WARN] Skipped installed package( %s => %s:%s )\n",
						n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))
					continue
				}
			}

			if !downloadCache[n.ImportPath] {
				// Download package.
				nod, imports := downloadPackage(n)
				if len(imports) > 0 {
					// Need to download dependencies.
					// Generate temporary nodes.
					nodes := make([]*doc.Node, len(imports))
					for i := range nodes {
						nodes[i] = &doc.Node{
							ImportPath:  imports[i],
							DownloadURL: imports[i],
							Type:        doc.BRANCH,
							IsGetDeps:   true,
						}
					}
					downloadPackages(nodes)
				}

				// Only save package information with specific commit.
				if nod != nil {
					// Save record in local nodes.
					com.ColorLog("[SUCC] Downloaded package( %s => %s:%s )\n",
						n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))
					downloadCount++
					saveNode(nod)
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

	imports, err := doc.PureDownload(nod, installRepoPath, CmdGet.Flags)

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
