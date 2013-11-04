package cmd

import (
	"github.com/Unknwon/com"
	"github.com/gpmgo/gopm/doc"
	"go/build"
	"os"
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

	err = getChildPkgs(curPath, nil, cachePkgs)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}

	newGoPath := filepath.Join(curPath, "vendor")
	os.RemoveAll(newGoPath)
	newGoPathSrc := filepath.Join(newGoPath, "src")
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

	if pkgName != "" {
		newPath := filepath.Join(newGoPathSrc, pkgName)
		com.ColorLog("[INFO] linked %v\n", pkgName)
		err = autoLink(curPath, newPath)
		if err != nil {
			com.ColorLog("[ERRO] make link error %v\n", err)
			return
		}
		com.ColorLog("[INFO] change dir to %v\n", newPath)
		err = os.Chdir(newPath)
		if err != nil {
			com.ColorLog("[ERRO] change current directory error %v\n", err)
			return
		}
	}

	com.ColorLog("[TRAC] set GOPATH=%v\n", newGoPath)
	err = os.Setenv("GOPATH", newGoPath)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return
	}
}
