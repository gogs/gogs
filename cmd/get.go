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
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var (
	installRepoPath string // The path of gopm local repository.
	installGopath   string // The first path in the GOPATH.
	isHasGopath     bool   // Indicates whether system has GOPATH.

	downloadCache map[string]bool // Saves packages that have been downloaded.
	downloadCount int
	failConut     int
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

If no version specified and package exists in GOPATH,
it will be skipped unless user enabled '--remote, -r' option 
then all the packages go into gopm local repository.`,
	Action: runGet,
	Flags: []cli.Flag{
		cli.BoolFlag{"gopath, g", "download all pakcages to GOPATH"},
		cli.BoolFlag{"update, u", "update pakcage(s) and dependencies if any"},
		cli.BoolFlag{"example, e", "download dependencies for example folder"},
		cli.BoolFlag{"remote, r", "download all pakcages to gopm local repository"},
		cli.BoolFlag{"verbose, v", "show process details"},
	},
}

func init() {
	downloadCache = make(map[string]bool)
}

func runGet(ctx *cli.Context) {
	setup(ctx)

	// Check conflicts.
	if ctx.Bool("gopath") && ctx.Bool("remote") {
		log.Error("get", "Command options have conflicts")
		log.Error("", "Following options are not supposed to use at same time:")
		log.Error("", "\t'--gopath, -g' '--remote, -r'")
		log.Help("Try 'gopm help get' to get more information")
	}

	if !ctx.Bool("remote") {
		// Get GOPATH.
		installGopath = com.GetGOPATHs()[0]
		if com.IsDir(installGopath) {
			isHasGopath = true
			log.Log("Indicated GOPATH: %s", installGopath)
			installGopath += "/src"
		} else {
			if ctx.Bool("gopath") {
				log.Error("get", "Invalid GOPATH path")
				log.Error("", "GOPATH does not exist or is not a directory:")
				log.Error("", "\t"+installGopath)
				log.Help("Try 'go help gopath' to get more information")
			} else {
				// It's OK that no GOPATH setting
				// when user does not specify to use.
				log.Warn("No GOPATH setting available")
			}
		}
	}

	// The gopm local repository.
	installRepoPath = doc.HomeDir + "/repos"
	log.Log("Local repository path: %s", installRepoPath)

	// Check number of arguments to decide which function to call.
	switch len(ctx.Args()) {
	case 0:
		getByGopmfile(ctx)
	default:
		getByPath(ctx)
	}
}

func getByGopmfile(ctx *cli.Context) {
	// Check if gopmfile exists and generate one if not.
	if !com.IsFile(".gopmfile") {
		runGen(ctx)
	}
	gf := doc.NewGopmfile(".")

	// Get dependencies.
	imports := doc.GetAllImports([]string{workDir},
		parseTarget(gf.MustValue("target", "path")), ctx.Bool("example"))
	nodes := make([]*doc.Node, 0, len(imports))
	for _, p := range imports {
		node := doc.NewNode(p, p, doc.BRANCH, "", true)

		// Check if user specified the version.
		if v, err := gf.GetValue("deps", p); err == nil && len(v) > 0 {
			node.Type, node.Value = validPath(v)
		}
		nodes = append(nodes, node)
	}

	downloadPackages(ctx, nodes)
	doc.SaveLocalNodes()

	log.Log("%d package(s) downloaded, %d failed", downloadCount, failConut)
}

func getByPath(ctx *cli.Context) {
	nodes := make([]*doc.Node, 0, len(ctx.Args()))
	for _, info := range ctx.Args() {
		pkgPath := info
		node := doc.NewNode(pkgPath, pkgPath, doc.BRANCH, "", true)

		if i := strings.Index(info, "@"); i > -1 {
			pkgPath = info[:i]
			tp, ver := validPath(info[i+1:])
			node = doc.NewNode(pkgPath, pkgPath, tp, ver, true)
		}

		// Check package name.
		if !strings.Contains(pkgPath, "/") {
			pkgPath = doc.GetPkgFullPath(pkgPath)
		}

		nodes = append(nodes, node)
	}

	downloadPackages(ctx, nodes)
	doc.SaveLocalNodes()

	log.Log("%d package(s) downloaded, %d failed", downloadCount, failConut)
}

func copyToGopath(srcPath, destPath string) {
	importPath := strings.TrimPrefix(destPath, installGopath+"/")
	if len(getVcsName(destPath)) > 0 {
		log.Warn("Package in GOPATH has version control: %s", importPath)
		return
	}

	os.RemoveAll(destPath)
	err := com.CopyDir(srcPath, destPath)
	if err != nil {
		log.Error("download", "Fail to copy to GOPATH:")
		log.Fatal("", "\t"+err.Error())
	}

	log.Log("Package copied to GOPATH: %s", importPath)
}

// downloadPackages downloads packages with certain commit,
// if the commit is empty string, then it downloads all dependencies,
// otherwise, it only downloada package with specific commit only.
func downloadPackages(ctx *cli.Context, nodes []*doc.Node) {
	// Check all packages, they may be raw packages path.
	for _, n := range nodes {
		// Check if local reference
		if n.Type == doc.LOCAL {
			continue
		}
		// Check if it is a valid remote path.
		if doc.IsValidRemotePath(n.ImportPath) {
			gopathDir := path.Join(installGopath, n.ImportPath)
			n.RootPath = doc.GetProjectPath(n.ImportPath)
			installPath := path.Join(installRepoPath, n.RootPath) +
				versionSuffix(n.Value)

			if !ctx.Bool("update") {
				// Check if package has been downloaded.
				if (len(n.Value) == 0 && !ctx.Bool("remote") && com.IsExist(gopathDir)) ||
					com.IsExist(installPath) {
					log.Trace("Skipped installed package: %s@%s:%s",
						n.ImportPath, n.Type, doc.CheckNodeValue(n.Value))

					if ctx.Bool("gopath") && com.IsExist(installPath) {
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
						log.Log("Found gopmfile: %s@%s:%s",
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
							nodes[i].Type, nodes[i].Value = validPath(v)
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

					if ctx.Bool("gopath") && com.IsExist(installPath) && !ctx.Bool("update") &&
						len(getVcsName(path.Join(installGopath, nod.RootPath))) == 0 {
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
	downloadCache[nod.RootPath] = true

	// Check if only need to use VCS tools.
	var imports []string
	var err error
	gopathDir := path.Join(installGopath, nod.RootPath)
	vcs := getVcsName(gopathDir)
	if ctx.Bool("update") && ctx.Bool("gopath") && len(vcs) > 0 {
		err = updateByVcs(vcs, gopathDir)
		imports = doc.GetAllImports([]string{gopathDir}, nod.RootPath, false)
	} else {
		nod.Revision = doc.LocalNodes.MustValue(nod.RootPath, "value")
		imports, err = doc.PureDownload(nod, installRepoPath, ctx) //CmdGet.Flags)
	}

	if err != nil {
		log.Error("get", "Fail to download pakage: "+nod.ImportPath)
		log.Error("", "\t"+err.Error())
		failConut++
		os.RemoveAll(installRepoPath + "/" + nod.RootPath)
		return nil, nil
	}
	return nod, imports
}

func getVcsName(dirPath string) string {
	switch {
	case com.IsExist(path.Join(dirPath, ".git")):
		return "git"
	case com.IsExist(path.Join(dirPath, ".hg")):
		return "hg"
	case com.IsExist(path.Join(dirPath, ".svn")):
		return "svn"
	}
	return ""
}

func updateByVcs(vcs, dirPath string) error {
	err := os.Chdir(dirPath)
	if err != nil {
		log.Error("Update by VCS", "Fail to change work directory:")
		log.Fatal("", "\t"+err.Error())
	}
	defer os.Chdir(workDir)

	switch vcs {
	case "git":
		stdout, _, err := com.ExecCmd("git", "status")
		if err != nil {
			log.Error("", "Error occurs when 'git status'")
			log.Error("", "\t"+err.Error())
		}

		i := strings.Index(stdout, "\n")
		if i == -1 {
			log.Error("", "Empty result for 'git status'")
			return nil
		}

		branch := strings.TrimPrefix(stdout[:i], "# On branch ")
		_, _, err = com.ExecCmd("git", "pull", "origin", branch)
		if err != nil {
			log.Error("", "Error occurs when 'git pull origin "+branch+"'")
			log.Error("", "\t"+err.Error())
		}
	case "hg":
		_, stderr, err := com.ExecCmd("hg", "pull")
		if err != nil {
			log.Error("", "Error occurs when 'hg pull'")
			log.Error("", "\t"+err.Error())
		}
		if len(stderr) > 0 {
			log.Error("", "Error: "+stderr)
		}

		_, stderr, err = com.ExecCmd("hg", "up")
		if err != nil {
			log.Error("", "Error occurs when 'hg up'")
			log.Error("", "\t"+err.Error())
		}
		if len(stderr) > 0 {
			log.Error("", "Error: "+stderr)
		}
	}
	return nil
}
