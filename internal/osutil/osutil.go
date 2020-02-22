// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package osutil

import (
	"os"
)

// IsFile returns true if given path exists as a file (i.e. not a directory).
func IsFile(path string) bool {
	f, e := os.Stat(path)
	if e != nil {
		return false
	}
	return !f.IsDir()
}

// CurrentUsername returns the current system user via environment variables.
func CurrentUsername() string {
	curUserName := os.Getenv("USER")
	if len(curUserName) > 0 {
		return curUserName
	}

	return os.Getenv("USERNAME")
}
