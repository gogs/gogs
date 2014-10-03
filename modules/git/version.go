// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"errors"
	"strings"

	"github.com/Unknwon/com"
)

var (
	// Cached Git version.
	gitVer *Version
)

// Version represents version of Git.
type Version struct {
	Major, Minor, Patch int
}

func ParseVersion(verStr string) (*Version, error) {
	infos := strings.Split(verStr, ".")
	if len(infos) < 3 {
		return nil, errors.New("incorrect version input")
	}

	v := &Version{}
	for i, s := range infos {
		switch i {
		case 0:
			v.Major, _ = com.StrTo(s).Int()
		case 1:
			v.Minor, _ = com.StrTo(s).Int()
		case 2:
			v.Patch, _ = com.StrTo(strings.TrimSpace(s)).Int()
		}
	}
	return v, nil
}

func MustParseVersion(verStr string) *Version {
	v, _ := ParseVersion(verStr)
	return v
}

// Compare compares two versions,
// it returns 1 if original is greater, -1 if original is smaller, 0 if equal.
func (v *Version) Compare(that *Version) int {
	if v.Major > that.Major {
		return 1
	} else if v.Major < that.Major {
		return -1
	}

	if v.Minor > that.Minor {
		return 1
	} else if v.Minor < that.Minor {
		return -1
	}

	if v.Patch > that.Patch {
		return 1
	} else if v.Patch < that.Patch {
		return -1
	}

	return 0
}

func (v *Version) LessThan(that *Version) bool {
	return v.Compare(that) < 0
}

func (v *Version) AtLeast(that *Version) bool {
	return v.Compare(that) >= 0
}

// GetVersion returns current Git version installed.
func GetVersion() (*Version, error) {
	if gitVer != nil {
		return gitVer, nil
	}

	stdout, stderr, err := com.ExecCmd("git", "version")
	if err != nil {
		return nil, errors.New(stderr)
	}

	infos := strings.Split(stdout, " ")
	if len(infos) < 3 {
		return nil, errors.New("not enough output")
	}

	gitVer, err = ParseVersion(infos[2])
	return gitVer, err
}
