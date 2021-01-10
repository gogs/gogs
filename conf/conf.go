// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"embed"

	"github.com/alimy/embedx"
)

var embedFS embedx.EmbedFS

func init() {
	//go:embed auth.d gitignore label license locale readme app.ini
	var content embed.FS

	embedFS = embedx.AttachRoot(content, "conf")
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	data, err := embedFS.ReadFile(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}
	return data
}

// Asset loads and returns the asset for the given name.
func Asset(name string) ([]byte, error) {
	return embedFS.ReadFile(name)
}

// AssetDir returns the file names below a certain directory.
func AssetDir(name string) ([]string, error) {
	entries, err := embedFS.ReadDir(name)
	if err != nil {
		return nil, err
	}
	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		fileNames = append(fileNames, entry.Name())
	}
	return fileNames, nil
}
