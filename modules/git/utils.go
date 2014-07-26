// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"container/list"
	"path/filepath"
	"strings"
)

const prettyLogFormat = `--pretty=format:%H`

func parsePrettyFormatLog(repo *Repository, logByts []byte) (*list.List, error) {
	l := list.New()
	if len(logByts) == 0 {
		return l, nil
	}

	parts := bytes.Split(logByts, []byte{'\n'})

	for _, commitId := range parts {
		commit, err := repo.GetCommit(string(commitId))
		if err != nil {
			return nil, err
		}
		l.PushBack(commit)
	}

	return l, nil
}

func RefEndName(refStr string) string {
	index := strings.LastIndex(refStr, "/")
	if index != -1 {
		return refStr[index+1:]
	}
	return refStr
}

// If the object is stored in its own file (i.e not in a pack file),
// this function returns the full path to the object file.
// It does not test if the file exists.
func filepathFromSHA1(rootdir, sha1 string) string {
	return filepath.Join(rootdir, "objects", sha1[:2], sha1[2:])
}
