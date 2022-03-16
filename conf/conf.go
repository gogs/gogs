// Copyright 2021 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed auth.d gitignore label license locale readme app.ini
var embedFS embed.FS

// trimPath maps name to the trim prefix "conf/" path.
func trimPath(name string) string {
	if name == "conf/" || name == "conf" {
		return "."
	}
	return strings.TrimPrefix(name, "conf/")
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	path := trimPath(name)
	data, err := fs.ReadFile(embedFS, path)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}
	return data
}

// Asset loads and returns the asset for the given name.
func Asset(name string) ([]byte, error) {
	path := trimPath(name)
	return fs.ReadFile(embedFS, path)
}

// AssetDir returns the file names below a certain directory.
func AssetDir(name string) ([]string, error) {
	path := trimPath(name)
	entries, err := fs.ReadDir(embedFS, path)
	if err != nil {
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}
