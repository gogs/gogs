// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"go/token"
	"os"
	"time"
)

// Node represents a node structure.
type Node struct {
	ImportPath string `json:"import_path"`
	Commit     string
	Date       string
}

// source is source code file.
type source struct {
	rawURL string
	name   string
	data   []byte
}

func (s *source) Name() string       { return s.name }
func (s *source) Size() int64        { return int64(len(s.data)) }
func (s *source) Mode() os.FileMode  { return 0 }
func (s *source) ModTime() time.Time { return time.Time{} }
func (s *source) IsDir() bool        { return false }
func (s *source) Sys() interface{}   { return nil }

// walker holds the state used when building the documentation.
type walker struct {
	ImportPath string
	srcs       map[string]*source // Source files.
	fset       *token.FileSet
}
