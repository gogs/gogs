// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"container/list"
	"os"
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

// isDir returns true if given path is a directory,
// or returns false when it's a file or does not exist.
func isDir(dir string) bool {
	f, e := os.Stat(dir)
	if e != nil {
		return false
	}
	return f.IsDir()
}

// isFile returns true if given path is a file,
// or returns false when it's a directory or does not exist.
func isFile(filePath string) bool {
	f, e := os.Stat(filePath)
	if e != nil {
		return false
	}
	return !f.IsDir()
}
