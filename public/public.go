// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package public

import (
	"embed"
	"net/http"
)

//go:embed assets css img js less plugins
var embedFS embed.FS

// NewFileSystem returns an http.FileSystem instance backed by embedded assets.
func NewFileSystem() http.FileSystem {
	return http.FS(embedFS)
}
