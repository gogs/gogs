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
	"path"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var (
	installRepoPath string
	installGopath   string
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
		cli.BoolFlag{"gopath, g", "download package(s) to GOPATH"},
		cli.BoolFlag{"force, f", "force to update pakcage(s) and dependencies"},
		cli.BoolFlag{"example, e", "download dependencies for example(s)"},
	},
}

func init() {
	downloadCache = make(map[string]bool)
}

func runGet(ctx *cli.Context) {
	doc.LoadPkgNameList(doc.HomeDir + "/data/pkgname.list")

	if ctx.Bool("gopath") {
		installGopath = com.GetGOPATHs()[0]
		if !com.IsDir(installGopath) {
			log.Error("Get", "Fail to start command")
			log.Fatal("", "GOPATH does not exist: "+installGopath)
		}
		log.Log("Indicate GOPATH: %s", installGopath)

		installGopath += "/src"
	}

	installRepoPath = doc.HomeDir + "/repos"
	log.Log("Local repository path: %s", installRepoPath)

	// Check number of arguments.
	switch len(ctx.Args()) {
	case 0:
		getByGopmfile(ctx)
	default:
		getByPath(ctx)
	}

}

func getByGopmfile(ctx *cli.Context) {
	if !com.IsFile(".gopmfile") {
		log.Fatal("Get", "No argument is supplied and no gopmfile exist")
	}

	gf := doc.NewGopmfile(".")

	absPath, err := filepath.Abs(".")
	if err != nil {
		log.Error("Get", "Fail to get absolute path of work directory")
		log.Fatal("", err.Error())
	}

	log.Log("Work directory: %s", absPath)

	// Get dependencies.
	imports := doc.GetAllImports([]string{absPath},
		gf.MustValue("target", "path"), ctx.Bool("example"))

	nodes := make([]*doc.Node, 0, len(imports))
	for _, p := range imports {
		node := doc.NewNode(p, p, doc.BRANCH, "", true)

		// Check if user specified the version.
		if v, err := gf.GetValue("deps", p); err == nil && len(v) > 0 {
			tp, ver, err := validPath(v)
			if err != nil {
				log.Error("", "Fail to parse version")
				log.Fatal("", err.Error())
			}
			node.Type = tp
			node.Value = ver
		}
		nodes = append(nodes, node)
	}

	downloadPackages(ctx, nodes)

	if doc.LocalNodes != nil {
		if err := goconfig.SaveConfigFile(doc.LocalNodes,
			doc.HomeDir+doc.LocalNodesFile); err != nil {
			log.Error("Get", "Fail to save localnodes.list")
		}
	}

	log.Log("%d package(s) downloaded, %d failed",
		downloadCount, failConut)
}

func getByPath(ctx *cli.Context) {
	nodes := make([]*doc.Node, 0, len(ctx.Args()))
	for _, info := range ctx.Args() {
		pkgName := info
		node := doc.NewNode(pkgName, pkgName, doc.BRANCH, "", true)

		if i := strings.Index(info, "@"); i > -1 {
			pkgName = info[:i]
			tp, ver, err := validPath(info[i+1:])
			if err != nil {
				log.Error("Get", "Fail to parse version")
				log.Fatal("", err.Error())
			}
			node = doc.NewNode(pkgName, pkgName, tp, ver, true)
		}

		// Check package name.
		if !strings.Contains(pkgName, "/") {
			name, ok := doc.PackageNameList[pkgName]
			if !ok {
				log.Error("Get", "Invalid package name: "+pkgName)
				log.Fatal("", "No match in the package name list")
			}
			pkgName = name
		}

		nodes = append(nodes, node)
	}

	downloadPackages(ctx, nodes)

	if doc.LocalNodes != nil {
		if err := goconfig.SaveConfigFile(doc.LocalNodes,
			doc.HomeDir+doc.LocalNodesFile); err != nil {
			log.Error("Get", "Fail to save localnodes.list")
		}
	}

	log.Log("%d package(s) downloaded, %d failed",
		downloadCount, failConut)
}

func copyToGopath(srcPath, destPath string) {
	fmt.Println(destPath)
	os.RemoveAll(destPath)
	err := com.CopyDir(srcPath, destPath)
	if err != nil {
		log.Error("Download", "Fail to copy to GOPATH")
		log.Fatal("", err.Error())
	}
}

// downloadPackages downloads packages with certain commit,
// if the commit is empty string, then it downloads all dependencies,
// otherwise, it only downloada package with specific commit only.
func downloadPackages(ctx *cli.Context, nodes []*doc.Node) {
	// Check all packages, they may be raw packages path.
	for _, n := range nodes {
		// Check if it is a valid remote path.
		if doc.IsValidRemotePath(n.ImportPath) {
			gopathDir := path.Join(installGopath, n.ImportPath)
			installPath := path.Join(installRepoPath, doc.GetProjectPath(n.ImportPath))
			if len(n.Value) > 0 {
				installPath += "." + n.Value
			}

			if !ctx.Bool("force") {
				// Check if package has been downloaded.
				if com.IsExist(installPath) {
					log.Trace("Skipped installed package: %s@%s:%s",
						n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))

					if ctx.Bool("gopath") {
						copyToGopath(installPath, gopathDir)
					}
					continue
				} else {
					doc.LocalNodes.SetValue(doc.GetProjectPath(n.ImportPath), "value", "")
				}
			}

			if !downloadCache[n.ImportPath] {
				// Download package.
				nod, imports := downloadPackage(ctx, n)
				if len(imports) > 0 {
					var gf *goconfig.ConfigFile

					// Check if has gopmfile
					if com.IsFile(installPath + "/" + doc.GopmFileName) {
						log.Log("Found gopmgile: %s@%s:%s",
							n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))

						gf = doc.NewGopmfile(installPath /* + "/.gopmfile"*/)
					}

					// Need to download dependencies.
					// Generate temporary nodes.
					nodes := make([]*doc.Node, len(imports))
					for i := range nodes {
						nodes[i] = doc.NewNode(imports[i], imports[i], doc.BRANCH, "", true)

						if gf == nil {
							continue
						}

						// Check if user specified the version.
						if v, err := gf.GetValue("deps", imports[i]); err == nil &&
							len(v) > 0 {
							tp, ver, err := validPath(v)
							if err != nil {
								log.Error("Download", "Fail to parse version")
								log.Fatal("", err.Error())
							}
							nodes[i].Type = tp
							nodes[i].Value = ver
						}
					}
					downloadPackages(ctx, nodes)
				}

				// Only save package information with specific commit.
				if nod != nil {
					// Save record in local nodes.
					log.Success("SUCC", "GET", fmt.Sprintf("%s@%s:%s",
						n.ImportPath, n.Type, doc.CheckNodeValue(n.Value)))
					downloadCount++

					// Only save non-commit node.
					if len(nod.Value) == 0 && len(nod.Revision) > 0 {
						doc.LocalNodes.SetValue(doc.GetProjectPath(nod.ImportPath), "value", nod.Revision)
					}

					if ctx.Bool("gopath") {
						copyToGopath(installPath, gopathDir)
					}
				}
			} else {
				log.Trace("Skipped downloaded package: %s@%s:%s",
					n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))
			}
		} else if n.ImportPath == "C" {
			continue
		} else {
			// Invalid import path.
			log.Error("", "Skipped invalid package: "+fmt.Sprintf("%s@%s:%s",
				n.ImportPath, n.Type, doc.CheckNodeValue(n.Value)))
			failConut++
		}
	}
}

// downloadPackage downloads package either use version control tools or not.
func downloadPackage(ctx *cli.Context, nod *doc.Node) (*doc.Node, []string) {
	log.Message("Downloading", fmt.Sprintf("package: %s@%s:%s",
		nod.ImportPath, nod.Type, doc.CheckNodeValue(nod.Value)))
	// Mark as donwloaded.
	downloadCache[nod.ImportPath] = true

	nod.Revision = doc.LocalNodes.MustValue(doc.GetProjectPath(nod.ImportPath), "value")
	imports, err := doc.PureDownload(nod, installRepoPath, ctx) //CmdGet.Flags)

	if err != nil {
		log.Error("Get", "Fail to download pakage: "+nod.ImportPath)
		log.Error("", err.Error())
		failConut++
		os.RemoveAll(installRepoPath + "/" + doc.GetProjectPath(nod.ImportPath) + "/")
		return nil, nil
	}
	return nod, imports
}

// validPath checks if the information of the package is valid.
func validPath(info string) (string, string, error) {
	infos := strings.SplitN(info, ":", 2)

	l := len(infos)
	switch {
	case l == 1:
		return doc.BRANCH, "", nil
	case l == 2:
		switch infos[1] {
		case doc.TRUNK, doc.MASTER, doc.DEFAULT:
			infos[1] = ""
		}
		return infos[0], infos[1], nil
	default:
		return "", "", errors.New("Invalid version information")
	}
}
