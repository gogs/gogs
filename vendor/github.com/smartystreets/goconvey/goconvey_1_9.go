// +build go1.9

// To work correctly with out of GOPATH modules, some functions needed to
// switch from using go/build to golang.org/x/tools/go/packages. But that
// package depends on changes to go/types that were introduced in Go 1.9. Since
// modules weren't introduced until Go 1.11, using
// golang.org/x/tools/go/packages can safely be restricted to users of Go 1.9
// or above.
package main

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
)

// This method exists because of a bug in the go cover tool that
// causes an infinite loop when you try to run `go test -cover`
// on a package that has an import cycle defined in one of it's
// test files. Yuck.
func testFilesImportTheirOwnPackage(packagePath string) bool {
	meta, err := packages.Load(
		&packages.Config{
			Mode:  packages.NeedName | packages.NeedImports,
			Tests: true,
		},
		packagePath,
	)
	if err != nil {
		return false
	}

	testPackageID := fmt.Sprintf("%s [%s.test]", meta[0], meta[0])

	for _, testPackage := range meta[1:] {
		if testPackage.ID != testPackageID {
			continue
		}

		for dependency := range testPackage.Imports {
			if dependency == meta[0].PkgPath {
				return true
			}
		}
		break
	}
	return false
}

func resolvePackageName(path string) string {
	pkg, err := packages.Load(
		&packages.Config{
			Mode: packages.NeedName,
		},
		path,
	)
	if err == nil {
		return pkg[0].PkgPath
	}

	nameArr := strings.Split(path, endGoPath)
	return nameArr[len(nameArr)-1]
}
