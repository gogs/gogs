// Copyright 2011 Gary Burd
// Copyright 2013 Unknown
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

package doc

import (
	"bytes"
	"errors"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/GPMGo/gpm/utils"
)

type sliceWriter struct{ p *[]byte }

func (w sliceWriter) Write(p []byte) (int, error) {
	*w.p = append(*w.p, p...)
	return len(p), nil
}

func (w *walker) readDir(dir string) ([]os.FileInfo, error) {
	if dir != w.ImportPath {
		panic("unexpected")
	}
	fis := make([]os.FileInfo, 0, len(w.srcs))
	for _, src := range w.srcs {
		fis = append(fis, src)
	}
	return fis, nil
}

func (w *walker) openFile(path string) (io.ReadCloser, error) {
	if strings.HasPrefix(path, w.ImportPath+"/") {
		if src, ok := w.srcs[path[len(w.ImportPath)+1:]]; ok {
			return ioutil.NopCloser(bytes.NewReader(src.data)), nil
		}
	}
	panic("unexpected")
}

func simpleImporter(imports map[string]*ast.Object, path string) (*ast.Object, error) {
	pkg := imports[path]
	if pkg == nil {
		// Guess the package name without importing it. Start with the last
		// element of the path.
		name := path[strings.LastIndex(path, "/")+1:]

		// Trim commonly used prefixes and suffixes containing illegal name
		// runes.
		name = strings.TrimSuffix(name, ".go")
		name = strings.TrimSuffix(name, "-go")
		name = strings.TrimPrefix(name, "go.")
		name = strings.TrimPrefix(name, "go-")
		name = strings.TrimPrefix(name, "biogo.")

		// It's also common for the last element of the path to contain an
		// extra "go" prefix, but not always. TODO: examine unresolved ids to
		// detect when trimming the "go" prefix is appropriate.

		pkg = ast.NewObj(ast.Pkg, name)
		pkg.Data = ast.NewScope(nil)
		imports[path] = pkg
	}
	return pkg, nil
}

// build gets imports from source files.
func (w *walker) build(srcs []*source) ([]string, error) {
	// Add source files to walker, I skipped references here.
	w.srcs = make(map[string]*source)
	for _, src := range srcs {
		w.srcs[src.name] = src
	}

	w.fset = token.NewFileSet()

	// Find the package and associated files.
	ctxt := build.Context{
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		CgoEnabled:    true,
		JoinPath:      path.Join,
		IsAbsPath:     path.IsAbs,
		SplitPathList: func(list string) []string { return strings.Split(list, ":") },
		IsDir:         func(path string) bool { panic("unexpected") },
		HasSubdir:     func(root, dir string) (rel string, ok bool) { panic("unexpected") },
		ReadDir:       func(dir string) (fi []os.FileInfo, err error) { return w.readDir(dir) },
		OpenFile:      func(path string) (r io.ReadCloser, err error) { return w.openFile(path) },
		Compiler:      "gc",
	}

	bpkg, err := ctxt.ImportDir(w.ImportPath, 0)
	// Continue if there are no Go source files; we still want the directory info.
	_, nogo := err.(*build.NoGoError)
	if err != nil {
		if nogo {
			err = nil
		} else {
			return nil, errors.New("doc.walker.build(): " + err.Error())
		}
	}

	// Parse the Go files

	files := make(map[string]*ast.File)
	for _, name := range append(bpkg.GoFiles, bpkg.CgoFiles...) {
		file, err := parser.ParseFile(w.fset, name, w.srcs[name].data, parser.ParseComments)
		if err != nil {
			//beego.Error("doc.walker.build():", err)
			continue
		}
		files[name] = file
	}

	var imports []string
	for _, v := range bpkg.Imports {
		// Skip strandard library.
		if !utils.IsGoRepoPath(v) && v != w.ImportPath {
			imports = append(imports, v)
		}
	}

	return imports, err
}
