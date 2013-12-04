package cmd

import (
	"errors"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

var isWindowsXP = false

func getGopmPkgs(dirPath string, isTest bool) (pkgs map[string]*doc.Pkg, err error) {
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		log.Error("", "Fail to get absolute path of work directory:")
		log.Fatal("", "\t"+err.Error())
	}

	var builds map[string]string

	if com.IsFile(absPath + "/" + doc.GOPM_FILE_NAME) {
		gf := doc.NewGopmfile(absPath)

		if builds, err = gf.GetSection("deps"); err != nil {
			builds = nil
		}
	}

	pkg, err := build.ImportDir(dirPath, build.AllowBinary)
	if err != nil {
		return map[string]*doc.Pkg{}, errors.New("Fail to get imports: " + err.Error())
	}

	pkgs = make(map[string]*doc.Pkg)
	var imports []string = pkg.Imports
	if isTest {
		imports = append(imports, pkg.TestImports...)
	}
	for _, name := range imports {
		if name == "C" {
			//panic("nonono")
			continue
		}
		if !doc.IsGoRepoPath(name) {
			if builds != nil {
				if info, ok := builds[name]; ok {
					// Check version.
					if i := strings.Index(info, ":"); i > -1 {
						pkgs[name] = &doc.Pkg{
							ImportPath: name,
							Type:       info[:i],
							Value:      info[i+1:],
						}
						continue
					}
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

func getChildPkgs(ctx *cli.Context, cpath string, ppkg *doc.Pkg, cachePkgs map[string]*doc.Pkg, isTest bool) error {
	var suf string
	if ppkg != nil {
		suf = versionSuffix(ppkg.Value)
	}
	pkgs, err := getGopmPkgs(cpath+suf, isTest)
	if err != nil {
		return errors.New("Fail to get gopmfile deps: " + err.Error())
	}
	for name, pkg := range pkgs {
		if !pkgInCache(name, cachePkgs) {
			var newPath string
			if !build.IsLocalImport(name) {

				suf := versionSuffix(pkg.Value)
				newPath = filepath.Join(installRepoPath, pkg.ImportPath)
				if len(pkg.Value) == 0 && !ctx.Bool("remote") {
					newPath = filepath.Join(installGopath, pkg.ImportPath)
				}
				if pkgName != "" && strings.HasPrefix(pkg.ImportPath, pkgName) {
					newPath = filepath.Join(curPath, pkg.ImportPath[len(pkgName)+1:]+suf)
				} else {
					if !com.IsExist(newPath + suf) {
						node := doc.NewNode(pkg.ImportPath, pkg.ImportPath,
							pkg.Type, pkg.Value, true)
						nodes := []*doc.Node{node}
						downloadPackages(ctx, nodes)
						// TODO: Should handler download failed
					}
				}
			} else {
				newPath, err = filepath.Abs(name)
				if err != nil {
					return err
				}
			}
			err = getChildPkgs(ctx, newPath, pkg, cachePkgs, false)
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
		log.Error("", "Fail to get work directory:")
		log.Fatal("", "\t"+err.Error())
	}

	log.Log("Changing work directory to %s", curPath)
	err = os.Chdir(curPath)
	if err != nil {
		log.Error("", "Fail to change work directory:")
		log.Fatal("", "\t"+err.Error())
	}
	defer func() {
		log.Log("Changing work directory back to %s", cwd)
		os.Chdir(cwd)
	}()

	err = os.Chdir(curPath)
	if err != nil {
		log.Error("", "Fail to change work directory:")
		log.Fatal("", "\t"+err.Error())
	}

	oldGoPath := os.Getenv("GOPATH")
	log.Log("Setting GOPATH to %s", gopath)

	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	err = os.Setenv("GOPATH", gopath+sep+oldGoPath)
	if err != nil {
		log.Error("", "Fail to setting GOPATH:")
		log.Fatal("", "\t"+err.Error())
	}
	defer func() {
		log.Log("Setting GOPATH back to %s", oldGoPath)
		os.Setenv("GOPATH", oldGoPath)
	}()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Log("===== application outputs start =====\n")

	err = cmd.Run()

	fmt.Println()
	log.Log("====== application outputs end ======")
	return err
}

func genNewGoPath(ctx *cli.Context, isTest bool) {
	var err error
	curPath, err = os.Getwd()
	if err != nil {
		log.Error("", "Fail to get work directory:")
		log.Fatal("", "\t"+err.Error())
	}

	installRepoPath = doc.HomeDir + "/repos"

	if com.IsFile(curPath + "/" + doc.GOPM_FILE_NAME) {
		log.Trace("Loading gopmfile...")
		gf := doc.NewGopmfile(curPath)

		var err error
		pkgName, err = gf.GetValue("target", "path")
		if err == nil {
			log.Log("Target name: %s", pkgName)
		}
	}

	if len(pkgName) == 0 {
		_, pkgName = filepath.Split(curPath)
	}

	cachePkgs := make(map[string]*doc.Pkg)
	err = getChildPkgs(ctx, curPath, nil, cachePkgs, isTest)
	if err != nil {
		log.Error("", "Fail to get child pakcages:")
		log.Fatal("", "\t"+err.Error())
	}

	newGoPath = filepath.Join(curPath, doc.VENDOR)
	newGoPathSrc := filepath.Join(newGoPath, "src")
	os.RemoveAll(newGoPathSrc)
	os.MkdirAll(newGoPathSrc, os.ModePerm)

	for name, pkg := range cachePkgs {
		suf := versionSuffix(pkg.Value)

		oldPath := filepath.Join(installRepoPath, name) + suf
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

		if !isExistP && (len(pkg.Value) > 0 || ctx.Bool("remote")) {
			log.Log("Linking %s", name+suf)
			err = autoLink(oldPath, newPath)
			if err != nil {
				log.Error("", "Fail to make link:")
				log.Fatal("", "\t"+err.Error())
			}
		}
	}

	newCurPath = filepath.Join(newGoPathSrc, pkgName)
	log.Log("Linking %s", pkgName)
	err = autoLink(curPath, newCurPath)
	if err != nil {
		log.Error("", "Fail to make link:")
		log.Fatal("", "\t"+err.Error())
	}
}
