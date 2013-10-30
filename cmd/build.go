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
	"github.com/Unknwon/com"
	"github.com/gpmgo/gopm/doc"
	"go/build"
	"os"
	"path/filepath"
	//"syscall"
	"os/exec"
	"strings"
)

var CmdBuild = &Command{
	UsageLine: "build",
	Short:     "build according a gopmfile",
	Long: `
build
`,
}

func init() {
	CmdBuild.Run = runBuild
	CmdBuild.Flags = map[string]bool{}
}

func printBuildPrompt(flag string) {
}

func getGopmPkgs(path string, inludeSys bool) (map[string]*doc.Pkg, error) {
	abs, err := filepath.Abs(doc.GopmFileName)
	if err != nil {
		return nil, err
	}

	// load import path
	gf := doc.NewGopmfile()
	if com.IsExist(abs) {
		err := gf.Load(abs)
		if err != nil {
			return nil, err
		}
	} else {
		sec := doc.NewSection()
		sec.Name = "build"
		gf.Sections[sec.Name] = sec
	}

	var builds *doc.Section
	var ok bool
	if builds, ok = gf.Sections["build"]; !ok {
		return nil, errors.New("no found build section\n")
	}

	pkg, err := build.ImportDir(path, build.AllowBinary)
	if err != nil {
		return map[string]*doc.Pkg{}, err
	}

	pkgs := make(map[string]*doc.Pkg)
	for _, name := range pkg.Imports {
		if inludeSys || !isStdPkg(name) {
			if dep, ok := builds.Deps[name]; ok {
				pkgs[name] = dep.Pkg
			} else {
				pkgs[name] = doc.NewDefaultPkg(name)
			}
		}
	}
	return pkgs, nil
}

func pkgInCache(name string, cachePkgs map[string]*doc.Pkg) bool {
	//pkgs := strings.Split(name, "/")
	_, ok := cachePkgs[name]
	return ok
}

func getChildPkgs(cpath string, ppkg *doc.Pkg, cachePkgs map[string]*doc.Pkg) error {
	pkgs, err := getGopmPkgs(cpath, false)
	if err != nil {
		return err
	}
	for name, pkg := range pkgs {
		if !pkgInCache(name, cachePkgs) {
			newPath := filepath.Join(installRepoPath, pkg.ImportPath)
			if !com.IsExist(newPath) {
				var t, ver string = doc.BRANCH, ""
				node := doc.NewNode(pkg.ImportPath, pkg.ImportPath, t, ver, true)
				//node := new(doc.Node)
				//node.Pkg = *pkg

				nodes := []*doc.Node{node}
				downloadPackages(nodes)
				// should handler download failed
			}
			err = getChildPkgs(newPath, pkg, cachePkgs)
			if err != nil {
				return err
			}
		}
	}
	if ppkg != nil {
		cachePkgs[ppkg.ImportPath] = ppkg
	}
	return nil
}

func makeLink(oldPath, newPath string) error {
	cmd := exec.Command("ln", "-s", oldPath, newPath)
	return cmd.Run()
}

func runBuild(cmd *Command, args []string) {
	curPath, err := os.Getwd()
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	hd, err := com.HomeDir()
	if err != nil {
		com.ColorLog("[ERRO] Fail to get current user[ %s ]\n", err)
		return
	}

	installRepoPath = strings.Replace(reposDir, "~", hd, -1)

	cachePkgs := make(map[string]*doc.Pkg)
	err = getChildPkgs(curPath, nil, cachePkgs)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	newGoPath := filepath.Join(curPath, "vendor")
	os.RemoveAll(newGoPath)
	newGoPathSrc := filepath.Join(newGoPath, "src")
	os.MkdirAll(newGoPathSrc, os.ModePerm)

	for name, pkg := range cachePkgs {
		oldPath := filepath.Join(installRepoPath, name)
		newPath := filepath.Join(newGoPathSrc, name)
		paths := strings.Split(name, "/")
		var isExistP bool
		for i := 0; i < len(paths)-1; i++ {
			pName := filepath.Join(paths[:len(paths)-1-i]...)
			if _, ok := cachePkgs[pName]; ok {
				isExistP = true
				break
			}
		}

		if !isExistP {
			pName := filepath.Join(paths[:len(paths)-1]...)
			newPPath := filepath.Join(newGoPathSrc, pName)
			com.ColorLog("[TRAC] create dirs %v\n", newPPath)
			os.MkdirAll(newPPath, os.ModePerm)

			com.ColorLog("[INFO] linked %v\n", name)

			err = makeLink(oldPath, newPath)

			if err != nil {
				com.ColorLog("[ERRO] make link error %v\n", err)
				return
			}
		}
	}

	gopath := build.Default.GOPATH
	com.ColorLog("[TRAC] set GOPATH=%v\n", newGoPath)
	err = os.Setenv("GOPATH", newGoPath)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	cmdArgs := []string{"go", "build"}
	cmdArgs = append(cmdArgs, args...)
	bCmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	bCmd.Stdout = os.Stdout
	bCmd.Stderr = os.Stderr
	err = bCmd.Run()
	if err != nil {
		com.ColorLog("[ERRO] build failed: %v\n", err)
		return
	}

	com.ColorLog("[TRAC] set GOPATH=%v\n", gopath)
	err = os.Setenv("GOPATH", gopath)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	com.ColorLog("[SUCC] build successfully!\n")
}
