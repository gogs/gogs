// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"go/token"
	"os"
	"time"
)

// Node represents a node.
type Node struct {
	ImportPath  string `json:"import_path"`
	Type, Value string
	Deps        []*Node `json:"-"` // Dependencies.
}

// Bundle represents a bundle.
type Bundle struct {
	Id        int64
	UserId    int64  `json:"user_id"`
	Name      string `json:"bundle_name"`
	Timestamp int64
	Comment   string
	Nodes     []*Node
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
