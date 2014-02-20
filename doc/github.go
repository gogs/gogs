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
	"path"
	"regexp"
	"strings"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/log"
)

var (
	githubPattern = regexp.MustCompile(`^github\.com/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]*)?$`)
)

func GetGithubCredentials() string {
	return "client_id=" + Cfg.MustValue("github", "client_id") +
		"&client_secret=" + Cfg.MustValue("github", "client_secret")
}

// getGithubDoc downloads tarball from github.com.
func getGithubDoc(client *http.Client, match map[string]string, installRepoPath string, nod *Node, ctx *cli.Context) ([]string, error) {
	match["cred"] = GetGithubCredentials()

	// Check downlaod type.
	switch nod.Type {
	case BRANCH:
		if len(nod.Value) == 0 {
			match["sha"] = MASTER

			// Only get and check revision with the latest version.
			var refs []*struct {
				Ref    string
				Url    string
				Object struct {
					Sha  string
					Type string
					Url  string
				}
			}

			err := com.HttpGetJSON(client, com.Expand("https://api.github.com/repos/{owner}/{repo}/git/refs?{cred}", match), &refs)
			if err != nil {
				log.Error("GET", "Fail to get revision")
				log.Error("", err.Error())
				break
			}

			var etag string
		COMMIT_LOOP:
			for _, ref := range refs {
				switch {
				case strings.HasPrefix(ref.Ref, "refs/heads/master"):
					etag = ref.Object.Sha
					break COMMIT_LOOP
				}
			}
			if etag == nod.Revision {
				log.Log("GET Package hasn't changed: %s", nod.ImportPath)
				return nil, nil
			}
			nod.Revision = etag

		} else {
			match["sha"] = nod.Value
		}
	case TAG, COMMIT:
		match["sha"] = nod.Value
	default:
		return nil, errors.New("Unknown node type: " + nod.Type)
	}

	// We use .zip here.
	// zip: https://github.com/{owner}/{repo}/archive/{sha}.zip
	// tarball: https://github.com/{owner}/{repo}/tarball/{sha}

	// Downlaod archive.
	p, err := com.HttpGetBytes(client, com.Expand("https://github.com/{owner}/{repo}/archive/{sha}.zip", match), nil)
	if err != nil {
		return nil, errors.New("Fail to donwload Github repo -> " + err.Error())
	}

	shaName := com.Expand("{repo}-{sha}", match)
	if nod.Type == "tag" {
		shaName = strings.Replace(shaName, "-v", "-", 1)
	}

	var installPath string
	if nod.ImportPath == nod.DownloadURL {
		suf := "." + nod.Value
		if len(suf) == 1 {
			suf = ""
		}
		projectPath := com.Expand("github.com/{owner}/{repo}", match)
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
		return nil, errors.New(nod.ImportPath + " -> new zip: " + err.Error())
	}

	dirs := make([]string, 0, 5)
	// Need to add root path because we cannot get from tarball.
	dirs = append(dirs, installPath+"/")
	for _, f := range r.File {
		absPath := strings.Replace(f.Name, shaName, installPath, 1)
		// Create diretory before create file.
		os.MkdirAll(path.Dir(absPath)+"/", os.ModePerm)

	compareDir:
		switch {
		case strings.HasSuffix(absPath, "/"): // Directory.
			// Check if current directory is example.
			if !(!ctx.Bool("example") && strings.Contains(absPath, "example")) {
				for _, d := range dirs {
					if d == absPath {
						break compareDir
					}
				}
				dirs = append(dirs, absPath)
			}
		default:
			// Get file from archive.
			r, err := f.Open()
			if err != nil {
				return nil, err
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

			// Set modify time.
			os.Chtimes(absPath, f.ModTime(), f.ModTime())
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
