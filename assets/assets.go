// Copyright 2021 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package assets

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"gogs.io/gogs/internal/osutil"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"
)

//go:embed conf public templates
var embedFS embed.FS

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

func mustNames(fsys fs.FS) []string {
	var names []string
	walkDirFunc := func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			names = append(names, path)
		}
		return nil
	}
	if err := fs.WalkDir(fsys, ".", walkDirFunc); err != nil {
		panic("assetNames failure: " + err.Error())
	}
	return names
}

// NewTemplateFileSystem returns a macaron.TemplateFileSystem instance for embedded assets.
// The argument "dir" can be used to serve subset of embedded assets. Template file
// found under the "customDir" on disk has higher precedence over embedded assets.
func NewTemplateFileSystem(dir, customDir string) macaron.TemplateFileSystem {
	if dir != "" && !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	var err error
	var files []macaron.TemplateFile
	tmplFS, err := fs.Sub(embedFS, "templates")
	if err != nil {
		log.Fatal("Failed to the subtree rooted at fsys's dir: %v", err)
	}
	names := mustNames(tmplFS)
	for _, name := range names {
		if !strings.HasPrefix(name, dir) {
			continue
		}

		// Check if corresponding custom file exists
		var data []byte
		fpath := path.Join(customDir, name)
		if osutil.IsFile(fpath) {
			data, err = ioutil.ReadFile(fpath)
		} else {
			data, err = fs.ReadFile(tmplFS, name)
		}
		if err != nil {
			log.Fatal("Failed to read file: %v", err)
		}

		name = strings.TrimPrefix(name, dir)
		ext := path.Ext(name)
		name = strings.TrimSuffix(name, ext)
		files = append(files, macaron.NewTplFile(name, data, ext))
	}
	return &fileSystem{files: files}
}

// NewPublicFileSystem returns a http.FileSystem instance backed by embedded assets.
func NewPublicFileSystem() http.FileSystem {
	publicFS, err := fs.Sub(embedFS, "public")
	if err != nil {
		log.Fatal("Failed to the subtree rooted at fsys's dir: %v", err)
	}
	return http.FS(publicFS)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	data, err := fs.ReadFile(embedFS, name)
	if err != nil {
		log.Fatal("Failed to read file: %v", err)
	}
	return data
}

// Asset loads and returns the asset for the given name.
func Asset(name string) ([]byte, error) {
	return fs.ReadFile(embedFS, name)
}

// AssetDir returns the file names below a certain directory.
func AssetDir(name string) ([]string, error) {
	entries, err := fs.ReadDir(embedFS, name)
	if err != nil {
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}

// MustAssetDir is like AssetDir but panics when AssetDir would return an error.
// It simplifies safe initialization of global variables.
func MustAssetDir(name string) []string {
	fileNames, err := AssetDir(name)
	if err != nil {
		log.Fatal("Failed to read directory: %v", err)
	}
	return fileNames
}
