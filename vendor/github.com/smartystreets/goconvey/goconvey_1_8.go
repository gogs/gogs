// +build !go1.9

// To work correctly with out of GOPATH modules, some functions needed to
// switch from using go/build to golang.org/x/tools/go/packages. But that
// package depends on changes to go/types that were introduced in Go 1.9. Since
// modules weren't introduced until Go 1.11, users of Go 1.8 or below can't be
// using modules, so they can continue to use go/build.

package main

import (
	"go/build"
	"strings"
)

// This method exists because of a bug in the go cover tool that
// causes an infinite loop when you try to run `go test -cover`
// on a package that has an import cycle defined in one of it's
// test files. Yuck.
func testFilesImportTheirOwnPackage(packagePath string) bool {
	meta, err := build.ImportDir(packagePath, build.AllowBinary)
	if err != nil {
		return false
	}

	for _, dependency := range meta.TestImports {
		if dependency == meta.ImportPath {
			return true
		}
	}
	return false
}

func resolvePackageName(path string) string {
	pkg, err := build.ImportDir(path, build.FindOnly)
	if err == nil {
		return pkg.ImportPath
	}

	nameArr := strings.Split(path, endGoPath)
	return nameArr[len(nameArr)-1]
}
