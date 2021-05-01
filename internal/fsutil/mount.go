// Copyright 2021 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// A MountFS is a file system with a Mount method.
type MountFS interface {
	fs.FS

	// Mount returns an FS corresponding to the subtree rooted at dir.
	Mount(dir string) (fs.FS, error)
}

// Mount returns an FS corresponding to the mounted point rooted at fsys's dir.
//
// If fs implements MountFS, Mount calls returns fsys.Mount(dir).
// Otherwise, if dir is ".", Mount returns fsys unchanged.
// Otherwise, Mount returns a new FS implementation mount that,
// in effect, implements mount.Open(dir) as fsys.Open(path.Join(name, dir)).
// The implementation also translates calls to ReadDir, ReadFile, and Glob appropriately.
func Mount(fsys fs.FS, dir string) (fs.FS, error) {
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "mount", Path: dir, Err: errors.New("invalid name")}
	}
	if dir == "." {
		return fsys, nil
	}
	if fsys, ok := fsys.(MountFS); ok {
		return fsys.Mount(dir)
	}
	mfs := &mntFS{fsys: fsys}
	if dir[len(dir)-1] != '/' {
		mfs.dir, mfs.noSlashDir = dir+"/", dir
	} else {
		mfs.dir, mfs.noSlashDir = dir, dir[:len(dir)-1]
	}
	return mfs, nil
}

type mntFS struct {
	fsys       fs.FS
	dir        string
	noSlashDir string
}

type file struct {
	fs.File
	dir string
}

type fileInfo struct {
	fs.FileInfo
	dir string
}

func (f file) State() (fs.FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return fileInfo{FileInfo: info, dir: f.dir}, nil
}

func (e fileInfo) Name() string {
	return path.Join(e.dir, e.FileInfo.Name())
}

// amendName maps name to the trim prefix dir name.
func (f *mntFS) amendName(op string, name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: op, Path: name, Err: errors.New("invalid name")}
	}
	if name == f.dir || name == f.noSlashDir {
		return ".", nil
	}
	fn := strings.TrimPrefix(name, f.dir)
	return fn, nil
}

// padding maps name, which should not start with f.dir, back to the prefix before f.dir.
func (f *mntFS) padding(name string) (rel string) {
	return path.Join(f.dir, name)
}

// fixErr shortens any reported names in PathErrors by stripping dir.
func (f *mntFS) fixErr(err error) error {
	if e, ok := err.(*fs.PathError); ok {
		if !strings.HasPrefix(e.Path, f.dir) {
			err = fmt.Errorf("path need prefix with %s: %w", f.dir, err)
		}
	}
	return err
}

func (f *mntFS) Open(name string) (fs.File, error) {
	amend, err := f.amendName("open", name)
	if err != nil {
		return nil, err
	}
	myFile, err := f.fsys.Open(amend)
	if err != nil {
		return nil, f.fixErr(err)
	}
	return file{File: myFile, dir: f.dir}, nil
}

func (f *mntFS) ReadDir(name string) ([]fs.DirEntry, error) {
	amend, err := f.amendName("read", name)
	if err != nil {
		return nil, err
	}
	dir, err := fs.ReadDir(f.fsys, amend)
	if err != nil {
		return nil, f.fixErr(err)
	}
	return dir, nil
}

func (f *mntFS) ReadFile(name string) ([]byte, error) {
	amend, err := f.amendName("read", name)
	if err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(f.fsys, amend)
	return data, f.fixErr(err)
}

func (f *mntFS) Glob(pattern string) ([]string, error) {
	// Check pattern is well-formed.
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	if pattern == "." {
		return []string{f.dir}, nil
	}
	list, err := fs.Glob(f.fsys, pattern)
	for i, name := range list {
		list[i] = f.padding(name)
	}
	return list, f.fixErr(err)
}
