// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"embed"
	"io/fs"

	"gogs.io/gogs/internal/fsutil"
)

var (
	//go:embed auth.d gitignore label license locale readme app.ini
	content embed.FS

	embedFS fs.FS
)

func init() {
	efs, err := fsutil.Mount(content, "conf")
	if err != nil {
		panic("failed to init embed config: " + err.Error())
	}
	embedFS = efs
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	data, err := fs.ReadFile(embedFS, name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
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
