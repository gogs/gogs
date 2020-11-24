// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package public

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gogs.io/gogs/internal/assets"
)

//go:generate go-bindata -nomemcopy -nometadata -pkg=public -ignore="\\.DS_Store|less" -prefix=../../../public -debug=false -o=public_gen.go ../../../public/...

/*
	This file is a modified version of https://github.com/go-bindata/go-bindata/pull/18.
*/

type fileInfo struct {
	name string
	size int64
}

func (d fileInfo) Name() string {
	return d.name
}

func (d fileInfo) Size() int64 {
	return d.size
}

func (d fileInfo) Mode() os.FileMode {
	return os.FileMode(0644) | os.ModeDir
}

func (d fileInfo) ModTime() time.Time {
	return time.Time{}
}

// IsDir return file whether a directory
func (d *fileInfo) IsDir() bool {
	return true
}

func (d fileInfo) Sys() interface{} {
	return nil
}

// file implements the http.File interface.
type file struct {
	name string
	*bytes.Reader

	children       []os.FileInfo
	childrenOffset int
}

func (f *file) Close() error {
	return nil
}

// ⚠️ WARNING: This method is not concurrent-safe.
func (f *file) Readdir(count int) ([]os.FileInfo, error) {
	if len(f.children) == 0 {
		return nil, os.ErrNotExist
	}

	if count <= 0 {
		return f.children, nil
	}

	if f.childrenOffset+count > len(f.children) {
		count = len(f.children) - f.childrenOffset
	}
	offset := f.childrenOffset
	f.childrenOffset += count
	return f.children[offset : offset+count], nil
}

func (f *file) Stat() (os.FileInfo, error) {
	childCount := len(f.children)
	if childCount != 0 {
		return &fileInfo{
			name: f.name,
			size: int64(childCount),
		}, nil
	}
	return AssetInfo(f.name)
}

// fileSystem implements the http.FileSystem interface.
type fileSystem struct{}

func (f *fileSystem) Open(name string) (http.File, error) {
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	// Attempt to get it as a file
	p, err := Asset(name)
	if err != nil && !assets.IsErrNotFound(err) {
		return nil, err
	} else if err == nil {
		return &file{
			name:   name,
			Reader: bytes.NewReader(p),
		}, nil
	}

	// Attempt to get it as a directory
	paths, err := AssetDir(name)
	if err != nil && !assets.IsErrNotFound(err) {
		return nil, err
	}

	infos := make([]os.FileInfo, len(paths))
	for i, path := range paths {
		path = filepath.Join(name, path)
		info, err := AssetInfo(path)
		if err != nil {
			if !assets.IsErrNotFound(err) {
				return nil, err
			}
			// Not found as a file, assume it's a directory.
			infos[i] = &fileInfo{name: path}
		} else {
			infos[i] = info
		}
	}
	return &file{
		name:     name,
		children: infos,
	}, nil
}

// NewFileSystem returns an http.FileSystem instance backed by embedded assets.
func NewFileSystem() http.FileSystem {
	return &fileSystem{}
}
