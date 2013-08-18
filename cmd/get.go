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
	"net/http"
	"os/user"
	//"path"
	"regexp"
	"strings"

	"github.com/gpmgo/gopm/doc"
)

var (
	installRepoPath string
	downloadCache   map[string]bool // Saves packages that have been downloaded.
	downloadCount   int
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
		doc.ColorLog("[INFO] You enabled download without installing.\n")
	case "-u":
		doc.ColorLog("[INFO] You enabled force update.\n")
	case "-e":
		doc.ColorLog("[INFO] You enabled download dependencies of example(s).\n")
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
			doc.ColorLog("[ERRO] Unknown flag: %s.\n", f)
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
		doc.ColorLog("[ERROR] Please list the package that you want to install.\n")
		return
	}

	curUser, err := user.Current()
	if err != nil {
		doc.ColorLog("[ERROR] Fail to get current user[ %s ]\n", err)
		return
	}

	installRepoPath = strings.Replace(reposDir, "~", curUser.HomeDir, -1)
	doc.ColorLog("[INFO] Packages will be installed into( %s )\n", installRepoPath)

	nodes := []*doc.Node{}
	// ver describles branch, tag or commit.
	var t, ver string = doc.BRANCH, ""

	if len(args) >= 2 {
		t, ver, err = validPath(args[1])
		if err != nil {
			doc.ColorLog("[ERROR] Fail to parse 'args'[ %s ]\n", err)
			return
		}
	}

	nodes = append(nodes, &doc.Node{
		ImportPath: args[0],
		Type:       t,
		Value:      ver,
		IsGetDeps:  true,
	})

	// Download package(s).
	downloadPackages(nodes)

	doc.ColorLog("[INFO] %d package(s) downloaded.\n", downloadCount)
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
				installPath := installRepoPath + "/" + n.ImportPath
				if len(n.Value) > 0 {
					installPath += "." + n.Value
				}
				if doc.IsExist(installPath) {
					doc.ColorLog("[WARN] Skipped installed package( %s => %s:%s )\n",
						n.ImportPath, n.Type, n.Value)
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
							ImportPath: imports[i],
							Type:       doc.BRANCH,
							IsGetDeps:  true,
						}
					}
					downloadPackages(nodes)
				}

				// Only save package information with specific commit.
				if nod != nil {
					// Save record in local nodes.
					doc.ColorLog("[SUCC] Downloaded package( %s => %s:%s )\n",
						n.ImportPath, n.Type, n.Value)
					downloadCount++
					//saveNode(nod)
				}
			} else {
				doc.ColorLog("[WARN] Skipped downloaded package( %s => %s:%s )\n",
					n.ImportPath, n.Type, n.Value)
			}
		} else {
			// Invalid import path.
			doc.ColorLog("[WARN] Skipped invalid package path( %s => %s:%s )\n",
				n.ImportPath, n.Type, n.Value)
		}
	}
}

// downloadPackage downloads package either use version control tools or not.
func downloadPackage(nod *doc.Node) (*doc.Node, []string) {
	doc.ColorLog("[TRAC] Downloading package( %s => %s:%s )\n",
		nod.ImportPath, nod.Type, nod.Value)
	// Mark as donwloaded.
	downloadCache[nod.ImportPath] = true

	imports, err := pureDownload(nod)

	if err != nil {
		doc.ColorLog("[ERRO] Download falied[ %s ]\n", err)
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

// service represents a source code control service.
type service struct {
	pattern *regexp.Regexp
	prefix  string
	get     func(*http.Client, map[string]string, string, *doc.Node, map[string]bool) ([]string, error)
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{doc.GithubPattern, "github.com/", doc.GetGithubDoc},
	// {doc.GooglePattern, "code.google.com/", doc.GetGoogleDoc},
	// {doc.BitbucketPattern, "bitbucket.org/", doc.GetBitbucketDoc},
	// {doc.LaunchpadPattern, "launchpad.net/", doc.GetLaunchpadDoc},
}

// pureDownload downloads package without version control.
func pureDownload(nod *doc.Node) ([]string, error) {
	for _, s := range services {
		if s.get == nil || !strings.HasPrefix(nod.ImportPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(nod.ImportPath)
		if m == nil {
			if s.prefix != "" {
				return nil, errors.New("Cannot match package service prefix by given path")
			}
			continue
		}
		match := map[string]string{"importPath": nod.ImportPath}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.get(doc.HttpClient, match, installRepoPath, nod, CmdGet.Flags)
	}
	return nil, errors.New("Cannot match any package service by given path")
}

// func joinPath(paths ...string) string {
// 	if len(paths) < 1 {
// 		return ""
// 	}
// 	res := ""
// 	for _, p := range paths {
// 		res = path.Join(res, p)
// 	}
// 	return res
// }

// func download(url string, localfile string) error {
// 	fmt.Println("Downloading", url, "...")
// 	resp, err := http.Get(url)
// 	if err != nil {
// 		return err
// 	}
// 	defer resp.Body.Close()

// 	localdir := filepath.Dir(localfile)
// 	if !dirExists(localdir) {
// 		err = os.MkdirAll(localdir, 0777)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	if !fileExists(localfile) {
// 		f, err := os.Create(localfile)
// 		if err == nil {
// 			_, err = io.Copy(f, resp.Body)
// 		}
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// func extractPkg(pkg *Pkg, localfile string, update bool) error {
// 	fmt.Println("Extracting package", pkg.Name, "...")

// 	gopath := os.Getenv("GOPATH")
// 	var childDirs []string = strings.Split(pkg.Name, "/")

// 	if pkg.Ver != TRUNK {
// 		childDirs[len(childDirs)-1] = fmt.Sprintf("%v_%v_%v", childDirs[len(childDirs)-1], pkg.Ver, pkg.VerId)
// 	}
// 	dstDir := joinPath(gopath, "src", joinPath(childDirs...))
// 	//fmt.Println(dstDir)
// 	var err error
// 	if !update {
// 		if dirExists(dstDir) {
// 			return nil
// 		}
// 		err = os.MkdirAll(dstDir, 0777)
// 	} else {
// 		if dirExists(dstDir) {
// 			err = os.Remove(dstDir)
// 		} else {
// 			err = os.MkdirAll(dstDir, 0777)
// 		}
// 	}

// 	if err != nil {
// 		return err
// 	}

// 	if path.Ext(localfile) != ".zip" {
// 		return errors.New("Not implemented!")
// 	}

// 	r, err := zip.OpenReader(localfile)
// 	if err != nil {
// 		return err
// 	}
// 	defer r.Close()

// 	for _, f := range r.File {
// 		fmt.Printf("Contents of %s:\n", f.Name)
// 		if f.FileInfo().IsDir() {
// 			continue
// 		}

// 		paths := strings.Split(f.Name, "/")[1:]
// 		//fmt.Println(paths)
// 		if len(paths) < 1 {
// 			continue
// 		}

// 		if len(paths) > 1 {
// 			childDir := joinPath(dstDir, joinPath(paths[0:len(paths)-1]...))
// 			//fmt.Println("creating", childDir)
// 			err = os.MkdirAll(childDir, 0777)
// 			if err != nil {
// 				return err
// 			}
// 		}

// 		rc, err := f.Open()
// 		if err != nil {
// 			return err
// 		}

// 		newF, err := os.Create(path.Join(dstDir, joinPath(paths...)))
// 		if err == nil {
// 			_, err = io.Copy(newF, rc)
// 		}
// 		if err != nil {
// 			return err
// 		}
// 		rc.Close()
// 	}
// 	return nil
// }

// func getPackage(pkg *Pkg, url string) error {
// 	curUser, err := user.Current()
// 	if err != nil {
// 		return err
// 	}

// 	reposDir = strings.Replace(reposDir, "~", curUser.HomeDir, -1)
// 	localdir := path.Join(reposDir, pkg.Name)
// 	localdir, err = filepath.Abs(localdir)
// 	if err != nil {
// 		return err
// 	}

// 	localfile := path.Join(localdir, pkg.FileName())

// 	err = download(url, localfile)
// 	if err != nil {
// 		return err
// 	}

// 	return extractPkg(pkg, localfile, false)
// }

// func getDirect(pkg *Pkg) error {
// 	return getPackage(pkg, pkg.Url())
// }

/*func getFromSource(pkgName string, ver string, source string) error {
	urlTempl := "https://%v/%v"
	//urlTempl := "https://%v/archive/master.zip"
	url := fmt.Sprintf(urlTempl, source, pkgName)

	return getPackage(pkgName, ver, url)
}*/
