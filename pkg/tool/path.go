// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package tool

import (
	"path/filepath"
	"strings"
)

// IsSameSiteURLPath returns true if the URL path belongs to the same site, false otherwise.
// False: //url, http://url, /\url
// True: /url
func IsSameSiteURLPath(url string) bool {
	return len(url) >= 2 && url[0] == '/' && url[1] != '/' && url[1] != '\\'
}

// IsMaliciousPath returns true if given path is an absolute path or contains malicious content
// which has potential to traverse upper level directories.
func IsMaliciousPath(path string) bool {
	return filepath.IsAbs(path) || strings.Contains(path, "..")
}
