// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"go/token"
	"os"
	"time"
)

// Package represents a package.
type Package struct {
	// Package import path.
	ImportPath string
	AbsPath    string

	// Revision tag and project tags.
	Commit string

	// Imports.
	Imports []string

	// Directories.
	Dirs []string
}

// source is source code file.
type source struct {
	name      string
	browseURL string
	rawURL    string
	data      []byte
}

func (s *source) Name() string       { return s.name }
func (s *source) Size() int64        { return int64(len(s.data)) }
func (s *source) Mode() os.FileMode  { return 0 }
func (s *source) ModTime() time.Time { return time.Time{} }
func (s *source) IsDir() bool        { return false }
func (s *source) Sys() interface{}   { return nil }

// walker holds the state used when building the documentation.
type walker struct {
	pkg  *Package
	srcs map[string]*source // Source files.
	fset *token.FileSet
}
