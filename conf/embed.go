// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"embed"
)

//go:embed app.ini **/*
var Files embed.FS

// FileNames returns a list of filenames exists in the given direction within
// Files. The list includes names of subdirectories.
func FileNames(dir string) ([]string, error) {
	entries, err := Files.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}
