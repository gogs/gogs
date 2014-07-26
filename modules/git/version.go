// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"errors"
	"strings"

	"github.com/Unknwon/com"
)

// Version represents version of Git.
type Version struct {
	Major, Minor, Patch int
}

// GetVersion returns current Git version installed.
func GetVersion() (Version, error) {
	stdout, stderr, err := com.ExecCmd("git", "version")
	if err != nil {
		return Version{}, errors.New(stderr)
	}

	infos := strings.Split(stdout, " ")
	if len(infos) < 3 {
		return Version{}, errors.New("not enough output")
	}

	v := Version{}
	for i, s := range strings.Split(strings.TrimSpace(infos[2]), ".") {
		switch i {
		case 0:
			v.Major, _ = com.StrTo(s).Int()
		case 1:
			v.Minor, _ = com.StrTo(s).Int()
		case 2:
			v.Patch, _ = com.StrTo(s).Int()
		}
	}
	return v, nil
}
