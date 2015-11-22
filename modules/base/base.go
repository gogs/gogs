// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"os"
	"os/exec"
	"path/filepath"
)

const DOC_URL = "https://github.com/gogits/go-gogs-client/wiki"

type (
	TplName string
)

var GoGetMetas = make(map[string]bool)

// ExecPath returns the executable path.
func ExecPath() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	p, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	return p, nil
}
