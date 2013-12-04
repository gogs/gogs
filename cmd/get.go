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
	installRepoPath string          // The path of gopm local repository.
	installGopath   string          // The first path in the GOPATH.
	downloadCache   map[string]bool // Saves packages that have been downloaded.
	downloadCount   int
	failConut       int
)

var CmdGet = cli.Command{
	Name:  "get",
	Usage: "fetch remote package(s) and dependencies to local repository",
	Description: `Command get fetches a package, and any pakcage that it depents on. 
If the package has a gopmfile, the fetch process will be driven by that.

gopm get
gopm get <import path>@[<tag|commit|branch>:<value>]
gopm get <package name>@[<tag|commit|branch>:<value>]

Can specify one or more: gopm get beego@tag:v0.9.0 github.com/beego/bee

If no argument is supplied, then gopmfile must be present.
If no version specified and package exists in GOPATH,
it will be skipped unless user enabled '--remote, -r' option 
then all the packages go into gopm local repository.`,
	Action: runGet,
	Flags: []cli.Flag{
		cli.BoolFlag{"gopath, g", "download all pakcages to GOPATH"},
		cli.BoolFlag{"force, f", "force to update pakcage(s) and dependencies"},
		cli.BoolFlag{"example, e", "download dependencies for example folder"},
		cli.BoolFlag{"remote, r", "download all pakcages to gopm local repository"},
	},
}

func init() {
	downloadCache = make(map[string]bool)
}

func runGet(ctx *cli.Context) {
	// Check conflicts.
	if ctx.Bool("gopath") && ctx.Bool("remote") {
		log.Error("get", "Command options have conflicts")
		log.Error("", "Following options are not supposed to use at same time:")
		log.Error("", "\t'--gopath, -g' '--remote, -r'")
		log.Help("Try 'gopm help get' to get more information")
	}

	// Get GOPATH.
	installGopath = com.GetGOPATHs()[0]
	if !com.IsDir(installGopath) {
		log.Error("get", "Invalid GOPATH path")
		log.Error("", "GOPATH does not exist or is not a directory:")
		log.Error("", "\t"+installGopath)
		log.Help("Try 'go help gopath' to get more information")
	}
	log.Log("Indicated GOPATH: %s", installGopath)
	installGopath += "/src"

	// The gopm local repository.
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
		log.Error("get", "Gopmfile not found")
		log.Error("", "No argument is supplied and no gopmfile exists")
		log.Help("\n%s\n%s\n%s",
			"Work directory is supposed to have gopmfile when there is no argument supplied",
			"Try 'gopm gen' to auto-generate gopmfile",
			"Try 'gopm help gen' to get more information")
	}

	gf := doc.NewGopmfile(".")

	absPath, err := filepath.Abs(".")
	if err != nil {
		log.Error("get", "Fail to get absolute path of work directory")
		log.Fatal("", "\t"+err.Error())
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
				log.Error("get", "Cannot parse dependency version")
				log.Error("", err.Error()+":")
				log.Error("", "\t"+v)
				log.Help("Try 'gopm help get' to get more information")
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
			log.Error("get", "Fail to save localnodes.list:")
			log.Error("", "\t"+err.Error())
		}
	}

	log.Log("%d package(s) downloaded, %d failed",
		downloadCount, failConut)
}

func getByPath(ctx *cli.Context) {
	nodes := make([]*doc.Node, 0, len(ctx.Args()))
	for _, info := range ctx.Args() {
		pkgPath := info
		node := doc.NewNode(pkgPath, pkgPath, doc.BRANCH, "", true)

		if i := strings.Index(info, "@"); i > -1 {
			pkgPath = info[:i]
			tp, ver, err := validPath(info[i+1:])
			if err != nil {
				log.Error("get", "Cannot parse dependency version")
				log.Error("", err.Error()+":")
				log.Error("", "\t"+info[i+1:])
				log.Help("Try 'gopm help get' to get more information")
			}
			node = doc.NewNode(pkgPath, pkgPath, tp, ver, true)
		}

		// Check package name.
		if !strings.Contains(pkgPath, "/") {
			pkgPath = doc.GetPkgFullPath(pkgPath)
		}

		nodes = append(nodes, node)
	}

	downloadPackages(ctx, nodes)

	if doc.LocalNodes != nil {
		if err := goconfig.SaveConfigFile(doc.LocalNodes,
			doc.HomeDir+doc.LocalNodesFile); err != nil {
			log.Error("get", "Fail to save localnodes.list:")
			log.Error("", "\t"+err.Error())
		}
	}

	log.Log("%d package(s) downloaded, %d failed",
		downloadCount, failConut)
}

func copyToGopath(srcPath, destPath string) {
	os.RemoveAll(destPath)
	err := com.CopyDir(srcPath, destPath)
	if err != nil {
		log.Error("download", "Fail to copy to GOPATH:")
		log.Fatal("", "\t"+err.Error())
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
			n.RootPath = doc.GetProjectPath(n.ImportPath)
			installPath := path.Join(installRepoPath, n.RootPath) +
				versionSuffix(n.Value)

			if !ctx.Bool("force") {
				// Check if package has been downloaded.
				if (len(n.Value) == 0 && !ctx.Bool("remote") && com.IsExist(gopathDir)) ||
					com.IsExist(installPath) {
					log.Trace("Skipped installed package: %s@%s:%s",
						n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))

					if ctx.Bool("gopath") {
						copyToGopath(installPath, gopathDir)
					}
					continue
				} else {
					doc.LocalNodes.SetValue(n.RootPath, "value", "")
				}
			}

			if !downloadCache[n.RootPath] {
				// Download package.
				nod, imports := downloadPackage(ctx, n)
				if len(imports) > 0 {
					var gf *goconfig.ConfigFile

					// Check if has gopmfile
					if com.IsFile(installPath + "/" + doc.GOPM_FILE_NAME) {
						log.Log("Found gopmgile: %s@%s:%s",
							n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))

						gf = doc.NewGopmfile(installPath)
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
								log.Error("download", "Cannot parse dependency version")
								log.Error("", err.Error()+":")
								log.Error("", "\t"+v)
								log.Help("Try 'gopm help get' to get more information")
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
						doc.LocalNodes.SetValue(nod.RootPath, "value", nod.Revision)
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
			log.Error("download", "Skipped invalid package: "+fmt.Sprintf("%s@%s:%s",
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

	nod.Revision = doc.LocalNodes.MustValue(nod.RootPath, "value")
	imports, err := doc.PureDownload(nod, installRepoPath, ctx) //CmdGet.Flags)

	if err != nil {
		log.Error("get", "Fail to download pakage: "+nod.ImportPath)
		log.Error("", "\t"+err.Error())
		failConut++
		os.RemoveAll(installRepoPath + "/" + nod.RootPath)
		return nil, nil
	}
	return nod, imports
}

// validPath checks if the information of the package is valid.
func validPath(info string) (string, string, error) {
	infos := strings.Split(info, ":")

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

func versionSuffix(value string) string {
	if len(value) > 0 {
		return "." + value
	}
	return ""
}
