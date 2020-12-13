// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package templates

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"path"
	"strings"

	"gogs.io/gogs/internal/osutil"
	"gopkg.in/macaron.v1"
)

//go:embed admin base explore inject mail org repo status user home.tmpl install.tmpl
var resouce embed.FS

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	data, err := resouce.ReadFile(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}
	return data
}

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

// assetNames returns the names of the assets.
func assetNames() []string {
	var names []string
	fs.WalkDir(resouce, ".", func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	return names
}

// NewTemplateFileSystem returns a macaron.TemplateFileSystem instance for embedded assets.
// The argument "dir" can be used to serve subset of embedded assets. Template file
// found under the "customDir" on disk has higher precedence over embedded assets.
func NewTemplateFileSystem(dir, customDir string) macaron.TemplateFileSystem {
	if dir != "" && !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	var files []macaron.TemplateFile
	names := assetNames()
	for _, name := range names {
		if !strings.HasPrefix(name, dir) {
			continue
		}

		// Check if corresponding custom file exists
		var err error
		var data []byte
		fpath := path.Join(customDir, name)
		if osutil.IsFile(fpath) {
			data, err = ioutil.ReadFile(fpath)
		} else {
			data, err = resouce.ReadFile(name)
		}
		if err != nil {
			panic(err)
		}

		name = strings.TrimPrefix(name, dir)
		ext := path.Ext(name)
		name = strings.TrimSuffix(name, ext)
		files = append(files, macaron.NewTplFile(name, data, ext))
	}
	return &fileSystem{files: files}
}

