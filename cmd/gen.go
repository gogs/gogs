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
	"github.com/Unknwon/com"
	"go/build"
	"os"
	"path"
	"runtime"
	"strings"
)

var CmdGen = &Command{
	UsageLine: "gen [.gopmfile]",
	Short:     "generate a gopmfile according current go project",
	Long: `
generate a gopmfile according current go project
`,
}

func init() {
	CmdGen.Run = runGen
	CmdGen.Flags = map[string]bool{}
}

func printGenPrompt(flag string) {
}

func isStdPkg(pkgName string) bool {
	return com.IsExist(path.Join(path.Join(runtime.GOROOT(), "src/pkg"), pkgName))
}

func getPkgs(path string, inludeSys bool) ([]string, error) {
	pkg, err := build.ImportDir(path, build.AllowBinary)
	if err != nil {
		return []string{}, err
	}

	if inludeSys {
		return pkg.Imports, nil
	} else {
		pkgs := make([]string, 0)
		for _, name := range pkg.Imports {
			if !isStdPkg(name) {
				pkgs = append(pkgs, name)
			}
		}
		return pkgs, nil
	}
}

// scan a directory and gen a gopm file
func runGen(cmd *Command, args []string) {

	var gopmFile string = ".gopmfile"
	if len(args) > 0 {
		gopmFile = args[0]
	}

	curPath, err := os.Getwd()
	if err != nil {
		com.ColorLog("[ERRO] %v.\n", err)
		return
	}

	gopmPath := path.Join(curPath, gopmFile)

	if com.IsExist(gopmPath) {
		com.ColorLog("[WARN] %v already existed.\n", gopmFile)
		return
	}

	// search the project and gen gopmfile
	pkgs, err := getPkgs(curPath, false)
	if err != nil {
		com.ColorLog("[ERRO] %v.\n", err)
		return
	}

	f, err := os.OpenFile(gopmPath, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		com.ColorLog("[ERRO] %v.\n", err)
		return
	}
	defer f.Close()
	contents := "[build]\n" + strings.Join(pkgs, "\n")

	_, err = f.WriteString(contents)
	if err != nil {
		com.ColorLog("[ERRO] %v.\n", err)
		return
	}
	com.ColorLog("[INFO] %v generated successfully.\n", gopmFile)
}
