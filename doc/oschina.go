// Copyright 2013 gopm authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package doc

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/Unknwon/com"
)

var (
	oscTagRe   = regexp.MustCompile(`/repository/archive\?ref=(.*)">`)
	oscPattern = regexp.MustCompile(`^git\.oschina\.net/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]*)?$`)
)

// getGithubDoc downloads tarball from git.oschina.com.
func getOSCDoc(client *http.Client, match map[string]string, installRepoPath string, nod *Node, cmdFlags map[string]bool) ([]string, error) {
	// Check downlaod type.
	switch nod.Type {
	case BRANCH:
		if len(nod.Value) == 0 {
			match["sha"] = MASTER
		} else {
			match["sha"] = nod.Value
		}
	case TAG, COMMIT:
		match["sha"] = nod.Value
	default:
		return nil, errors.New("Unknown node type: " + nod.Type)
	}

	// zip: http://{projectRoot}/repository/archive?ref={sha}

	// Downlaod archive.
	p, err := com.HttpGetBytes(client, com.Expand("http://git.oschina.net/{owner}/{repo}/repository/archive?ref={sha}", match), nil)
	if err != nil {
		return nil, errors.New("Fail to donwload OSChina repo -> " + err.Error())
	}

	var installPath string
	if nod.ImportPath == nod.DownloadURL {
		suf := "." + nod.Value
		if len(suf) == 1 {
			suf = ""
		}
		projectPath := com.Expand("git.oschina.net/{owner}/{repo}", match)
		installPath = installRepoPath + "/" + projectPath + suf
		nod.ImportPath = projectPath
	} else {
		installPath = installRepoPath + "/" + nod.ImportPath
	}

	// Remove old files.
	os.RemoveAll(installPath + "/")
	os.MkdirAll(installPath+"/", os.ModePerm)

	r, err := zip.NewReader(bytes.NewReader(p), int64(len(p)))
	if err != nil {
		return nil, errors.New("Fail to unzip OSChina repo -> " + err.Error())
	}

	nameLen := len(match["repo"])
	dirs := make([]string, 0, 5)
	// Need to add root path because we cannot get from tarball.
	dirs = append(dirs, installPath+"/")
	for _, f := range r.File {
		fileName := f.Name[nameLen+1:]
		absPath := installPath + "/" + fileName

		if strings.HasSuffix(absPath, "/") {
			dirs = append(dirs, absPath)
			os.MkdirAll(absPath, os.ModePerm)
			continue
		}

		// Get file from archive.
		r, err := f.Open()
		if err != nil {
			return nil, errors.New("Fail to open OSChina repo -> " + err.Error())
		}

		fbytes := make([]byte, f.FileInfo().Size())
		_, err = io.ReadFull(r, fbytes)
		if err != nil {
			return nil, err
		}

		_, err = com.SaveFile(absPath, fbytes)
		if err != nil {
			return nil, err
		}
	}

	var imports []string

	// Check if need to check imports.
	if nod.IsGetDeps {
		for _, d := range dirs {
			importPkgs, err := CheckImports(d, match["importPath"], nod)
			if err != nil {
				return nil, err
			}
			imports = append(imports, importPkgs...)
		}
	}

	return imports, err
}
