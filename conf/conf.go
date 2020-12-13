// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"embed"
	"io/fs"
	"strings"
)

var (
	//go:embed auth.d gitignore label license locale readme app.ini
	resouce embed.FS

	emFS embedFS
)

func init() {
	emFS = newEmbedFS("conf", resouce)
}

type embedFS struct {
	*embed.FS
	prefix      string
	prefixSlash string
}

func newEmbedFS(prefix string, resouce embed.FS) embedFS {
	prefix = strings.TrimPrefix(prefix, "/")
	return embedFS{
		FS:          &resouce,
		prefix:      prefix,
		prefixSlash: prefix + "/",
	}
}

// Open opens the named file for reading and returns it as an fs.File.
func (f embedFS) Open(name string) (fs.File, error) {
	return f.FS.Open(f.trimPrefix(name))
}

// ReadDir reads and returns the entire named directory.
func (f embedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return f.FS.ReadDir(f.trimPrefix(name))
}

// ReadFile reads and returns the content of the named file.
func (f embedFS) ReadFile(name string) ([]byte, error) {
	return f.FS.ReadFile(f.trimPrefix(name))
}

func (f embedFS) trimPrefix(name string) string {
	if name == f.prefix {
		return "."
	}
	return strings.TrimPrefix(name, f.prefixSlash)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	data, err := emFS.ReadFile(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}
	return data
}

// Asset loads and returns the asset for the given name.
func Asset(name string) ([]byte, error) {
	return emFS.ReadFile(name)
}

// AssetDir returns the file names below a certain directory.
func AssetDir(name string) ([]string, error) {
	entries, err := emFS.ReadDir(name)
	if err != nil {
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}
