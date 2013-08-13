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
	"archive/zip"
	//"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"

	"../doc"
)

var (
	installGOPATH string // The GOPATH that packages are downloaded to.
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

func isStandalone() bool {
	return true
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
		doc.ColorLog("[ERRO] Please list the package that you want to install.\n")
		return
	}

	if len(args) > 0 {
		var ver string = TRUNK
		if len(args) == 2 {
			ver = args[1]
		}
		pkg := NewPkg(args[0], ver)
		if isStandalone() {
			getDirect(pkg)
		} else {
			fmt.Println("Not implemented.")
			//getSource(pkgName)
		}
	}
}

func dirExists(dir string) bool {
	d, e := os.Stat(dir)
	switch {
	case e != nil:
		return false
	case !d.IsDir():
		return false
	}

	return true
}

func fileExists(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func download(url string, localfile string) error {
	fmt.Println("Downloading", url, "...")
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	localdir := filepath.Dir(localfile)
	if !dirExists(localdir) {
		err = os.MkdirAll(localdir, 0777)
		if err != nil {
			return err
		}
	}

	if !fileExists(localfile) {
		f, err := os.Create(localfile)
		if err == nil {
			_, err = io.Copy(f, resp.Body)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

/*func extractPkg(pkg *Pkg, update bool) error {
	gopath := os.Getenv("GOPATH")
	var childDirs []string = strings.Split(pkg.Name, "/")

	if pkg.Ver != TRUNK {
		childDirs[len(childDirs)-1] = fmt.Sprintf("%v_%v_%v", childDirs[len(childDirs)-1], pkg.Ver, pkg.VerId)
	}
	srcDir = path.Join(gopath, childDir...)

	if !update {
		if dirExists(srcDir) {
			return nil
		}
		err = os.MkdirAll(localdir, 0777)
		if err != nil {
			return err
		}
	} else {
		if dirExists(srcDir) {
			os.Remove(localdir)
		} else {
			err = os.MkdirAll(localdir, 0777)
			if err != nil {
				return err
			}
		}
	}

	// Iterate through the files in the archive,
	// printing some of their contents.
	for _, f := range r.File {
		fmt.Printf("Contents of %s:\n", f.Name)
		rc, err := f.Open()
		if err != nil {
			return err
		}

		_, err = io.Copy(os.Stdout, rc)
		if err != nil {
			return err
		}
		rc.Close()
	}
	return nil
}*/

func getPackage(pkg *Pkg, url string) error {
	curUser, err := user.Current()
	if err != nil {
		return err
	}

	reposDir = strings.Replace(reposDir, "~", curUser.HomeDir, -1)
	localdir := path.Join(reposDir, pkg.Name)
	localdir, err = filepath.Abs(localdir)
	if err != nil {
		return err
	}

	urls := strings.Split(url, ".")

	localfile := path.Join(localdir, fmt.Sprintf("%v.%v", pkg.VerSimpleString(), urls[len(urls)-1]))

	err = download(url, localfile)
	if err != nil {
		return err
	}

	r, err := zip.OpenReader(localfile)
	if err != nil {
		return err
	}
	defer r.Close()

	if pkg.Ver != TRUNK {
		return nil
	}

	//return extractPkg(pkg)
	return nil
}

func getDirect(pkg *Pkg) error {
	return getPackage(pkg, pkg.Source.PkgUrl(pkg.Name, pkg.VerString()))
}

/*func getFromSource(pkgName string, ver string, source string) error {
	urlTempl := "https://%v/%v"
	//urlTempl := "https://%v/archive/master.zip"
	url := fmt.Sprintf(urlTempl, source, pkgName)

	return getPackage(pkgName, ver, url)
}*/
