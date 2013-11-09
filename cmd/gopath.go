package cmd

import (
	"github.com/Unknwon/com"
	"github.com/gpmgo/gopm/doc"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func getGopmPkgs(path string, inludeSys bool) (map[string]*doc.Pkg, error) {
	abs, err := filepath.Abs(filepath.Join(path, doc.GopmFileName))
	if err != nil {
		return nil, err
	}

	// load import path
	gf := doc.NewGopmfile()
	var builds *doc.Section
	if com.IsExist(abs) {
		err := gf.Load(abs)
		if err != nil {
			return nil, err
		}
		var ok bool
		if builds, ok = gf.Sections["build"]; !ok {
			builds = nil
		}
	}

	pkg, err := build.ImportDir(path, build.AllowBinary)
	if err != nil {
		return map[string]*doc.Pkg{}, err
	}

	pkgs := make(map[string]*doc.Pkg)
	for _, name := range pkg.Imports {
		if inludeSys || !isStdPkg(name) {
			if builds != nil {
				if dep, ok := builds.Deps[name]; ok {
					pkgs[name] = dep.Pkg
					continue
				}
			}
			pkgs[name] = doc.NewDefaultPkg(name)
		}
	}
	return pkgs, nil
}

func pkgInCache(name string, cachePkgs map[string]*doc.Pkg) bool {
	//pkgs := strings.Split(name, "/")
	_, ok := cachePkgs[name]
	return ok
}

func autoLink(oldPath, newPath string) error {
	newPPath, _ := filepath.Split(newPath)
	os.MkdirAll(newPPath, os.ModePerm)
	return makeLink(oldPath, newPath)
}

func getChildPkgs(cpath string, ppkg *doc.Pkg, cachePkgs map[string]*doc.Pkg) error {
	pkgs, err := getGopmPkgs(cpath, false)
	if err != nil {
		return err
	}
	for name, pkg := range pkgs {
		if !pkgInCache(name, cachePkgs) {
			var newPath string
			if !build.IsLocalImport(name) {
				newPath = filepath.Join(installRepoPath, pkg.ImportPath)
				if pkgName != "" && strings.HasPrefix(pkg.ImportPath, pkgName) {
					newPath = filepath.Join(curPath, pkg.ImportPath[len(pkgName)+1:])
				} else {
					if !com.IsExist(newPath) {
						var t, ver string = doc.BRANCH, ""
						node := doc.NewNode(pkg.ImportPath, pkg.ImportPath, t, ver, true)
						nodes := []*doc.Node{node}
						downloadPackages(nodes)
						// should handler download failed
					}
				}
			} else {
				newPath, err = filepath.Abs(name)
				if err != nil {
					return err
				}
			}
			err = getChildPkgs(newPath, pkg, cachePkgs)
			if err != nil {
				return err
			}
		}
	}
	if ppkg != nil && !build.IsLocalImport(ppkg.ImportPath) {
		cachePkgs[ppkg.ImportPath] = ppkg
	}
	return nil
}

var pkgName string
var curPath string
var newCurPath string
var newGoPath string

func execCmd(gopath, curPath string, args ...string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	com.ColorLog("[INFO] change current dir from %v to %v\n", cwd, curPath)
	err = os.Chdir(filepath.Join(cwd, "vendor"))
	if err != nil {
		com.ColorLog("[ERRO] change current directory error %v\n", err)
		return err
	}
	err = os.Chdir(curPath)
	if err != nil {
		com.ColorLog("[ERRO] change current directory error %v\n", err)
		return err
	}
	defer os.Chdir(cwd)
	ccmd := exec.Command("cd", curPath)
	ccmd.Stdout = os.Stdout
	ccmd.Stderr = os.Stderr
	err = ccmd.Run()
	if err != nil {
		com.ColorLog("[ERRO] change current directory error %v\n", err)
		return err
	}

	oldGoPath := os.Getenv("GOPATH")
	com.ColorLog("[TRAC] set GOPATH from %v to %v\n", oldGoPath, gopath)

	err = os.Setenv("GOPATH", gopath)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return err
	}
	defer os.Setenv("GOPATH", oldGoPath)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func genNewGoPath() {
	var err error
	curPath, err = os.Getwd()
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	hd, err := com.HomeDir()
	if err != nil {
		com.ColorLog("[ERRO] Fail to get current user[ %s ]\n", err)
		return
	}

	gf := doc.NewGopmfile()
	gpmPath := filepath.Join(curPath, doc.GopmFileName)
	if com.IsExist(gpmPath) {
		com.ColorLog("[INFO] loading .gopmfile ...\n")
		err := gf.Load(gpmPath)
		if err != nil {
			com.ColorLog("[ERRO] load .gopmfile failed: %v\n", err)
			return
		}
	}

	installRepoPath = strings.Replace(reposDir, "~", hd, -1)

	cachePkgs := make(map[string]*doc.Pkg)
	if target, ok := gf.Sections["target"]; ok {
		pkgName = target.Props["path"]
		com.ColorLog("[INFO] target name is %v\n", pkgName)
	}

	if pkgName == "" {
		_, pkgName = filepath.Split(curPath)
	}

	err = getChildPkgs(curPath, nil, cachePkgs)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	newGoPath = filepath.Join(curPath, "vendor")
	newGoPathSrc := filepath.Join(newGoPath, "src")
	os.RemoveAll(newGoPathSrc)
	os.MkdirAll(newGoPathSrc, os.ModePerm)

	for name, _ := range cachePkgs {
		oldPath := filepath.Join(installRepoPath, name)
		newPath := filepath.Join(newGoPathSrc, name)
		paths := strings.Split(name, "/")
		var isExistP bool
		var isCurChild bool
		for i := 0; i < len(paths)-1; i++ {
			pName := strings.Join(paths[:len(paths)-1-i], "/")
			if _, ok := cachePkgs[pName]; ok {
				isExistP = true
				break
			}
			if pkgName == pName {
				isCurChild = true
				break
			}
		}
		if isCurChild {
			continue
		}

		if !isExistP {
			com.ColorLog("[INFO] linked %v\n", name)
			err = autoLink(oldPath, newPath)
			if err != nil {
				com.ColorLog("[ERRO] make link error %v\n", err)
				return
			}
		}
	}

	newCurPath = filepath.Join(newGoPathSrc, pkgName)
	com.ColorLog("[INFO] linked %v\n", pkgName)
	err = autoLink(curPath, newCurPath)
	if err != nil {
		com.ColorLog("[ERRO] make link error %v\n", err)
		return
	}
}
