// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package templates

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	"gopkg.in/macaron.v1"
)

//go:generate go-bindata -nomemcopy -ignore="\\.DS_Store" -pkg=templates -prefix=../../../templates -debug=false -o=templates_gen.go ../../../templates/...

// fileSystem implements the macaron.TemplateFileSystem interface.
type fileSystem struct {
	files []macaron.TemplateFile
}

func (fs *fileSystem) ListFiles() []macaron.TemplateFile {
	return fs.files
}

func (fs *fileSystem) Get(name string) (io.Reader, error) {
	for i := range fs.files {
		if fs.files[i].Name()+fs.files[i].Ext() == name {
			return bytes.NewReader(fs.files[i].Data()), nil
		}
	}
	return nil, fmt.Errorf("file %q not found", name)
}

// NewTemplateFileSystem returns a macaron.TemplateFileSystem instance backed by embedded assets.
func NewTemplateFileSystem() macaron.TemplateFileSystem {
	names := AssetNames()
	fs := &fileSystem{
		files: make([]macaron.TemplateFile, len(names)),
	}

	for i, name := range names {
		p, err := Asset(name)
		if err != nil {
			panic(err)
		}

		ext := path.Ext(name)
		name = strings.TrimSuffix(name, ext)
		fs.files[i] = macaron.NewTplFile(name, p, ext)
	}
	return fs
}
