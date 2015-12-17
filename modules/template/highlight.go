// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package template

import (
	"path"
	"strings"
)

var (
	// File name should ignore highlight.
	ignoreFileNames = map[string]bool{
		"license": true,
		"copying": true,
	}

	// File names that are representing highlight class.
	highlightFileNames = map[string]bool{
		"dockerfile": true,
		"makefile":   true,
	}

	// Extensions that are same as highlight class.
	highlightExts = map[string]bool{
		".arm":    true,
		".as":     true,
		".sh":     true,
		".cs":     true,
		".cpp":    true,
		".c":      true,
		".css":    true,
		".cmake":  true,
		".bat":    true,
		".dart":   true,
		".patch":  true,
		".elixir": true,
		".erlang": true,
		".go":     true,
		".html":   true,
		".xml":    true,
		".hs":     true,
		".ini":    true,
		".json":   true,
		".java":   true,
		".js":     true,
		".less":   true,
		".lua":    true,
		".php":    true,
		".py":     true,
		".rb":     true,
		".scss":   true,
		".sql":    true,
		".scala":  true,
		".swift":  true,
		".ts":     true,
		".vb":     true,
	}
)

// FileNameToHighlightClass returns the best match for highlight class name
// based on the rule of highlight.js.
func FileNameToHighlightClass(fname string) string {
	fname = strings.ToLower(fname)
	if ignoreFileNames[fname] {
		return "nohighlight"
	}

	if highlightFileNames[fname] {
		return fname
	}

	ext := path.Ext(fname)
	if highlightExts[ext] {
		return ext[1:]
	}

	return ""
}
