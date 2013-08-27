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
)

var (
	githubRawHeader = http.Header{"Accept": {"application/vnd.github-blob.raw"}}
	githubPattern   = regexp.MustCompile(`^github\.com/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]*)?$`)
	githubCred      string
)

/*func SetGithubCredentials(id, secret string) {
	//githubCred = "client_id=" + id + "&client_secret=" + secret
}*/

func SetGithubCredentials(token string) {
	if len(token) > 0 {
		githubCred = "access_token=" + token
	}
}

// getGithubDoc downloads tarball from github.com.
func getGithubDoc(client *http.Client, match map[string]string, installRepoPath string, nod *Node, cmdFlags map[string]bool) ([]string, error) {
	match["cred"] = githubCred

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

	// We use .zip here.
	// zip: https://github.com/{owner}/{repo}/archive/{sha}.zip
	// tarball: https://github.com/{owner}/{repo}/tarball/{sha}

	// Downlaod archive.
	p, err := com.HttpGetBytes(client, expand("https://github.com/{owner}/{repo}/archive/{sha}.zip", match), nil)
	if err != nil {
		return nil, errors.New("Fail to donwload Github repo -> " + err.Error())
	}

	shaName := expand("{repo}-{sha}", match)
	if nod.Type == "tag" {
		shaName = strings.Replace(shaName, "-v", "-", 1)
	}

	var installPath string
	if nod.ImportPath == nod.DownloadURL {
		suf := "." + nod.Value
		if len(suf) == 1 {
			suf = ""
		}
		projectPath := expand("github.com/{owner}/{repo}", match)
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
		return nil, err
	}

	dirs := make([]string, 0, 5)
	// Need to add root path because we cannot get from tarball.
	dirs = append(dirs, installPath+"/")
	for _, f := range r.File {
		absPath := strings.Replace(f.FileInfo().Name(), shaName, installPath, 1)
		// Create diretory before create file.
		os.MkdirAll(path.Dir(absPath)+"/", os.ModePerm)

	compareDir:
		switch {
		case strings.HasSuffix(absPath, "/"): // Directory.
			// Check if current directory is example.
			if !(!cmdFlags["-e"] && strings.Contains(absPath, "example")) {
				for _, d := range dirs {
					if d == absPath {
						break compareDir
					}
				}
				dirs = append(dirs, absPath)
			}
		case !strings.HasPrefix(f.FileInfo().Name(), "."):
			// Get file from archive.
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}

			// Write data to file
			fw, _ := os.Create(absPath)
			if err != nil {
				return nil, err
			}

			_, err = io.Copy(fw, rc)
			// Close files.
			rc.Close()
			fw.Close()
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

	/*fpath := appPath + "repo/tarballs/" + node.ImportPath + "-" + node.Value + ".zip"
	// Save tarball.
	if autoBackup && !utils.IsExist(fpath) {
		os.MkdirAll(path.Dir(fpath)+"/", os.ModePerm)
		f, err := os.Create(fpath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		_, err = f.Write(p)
	}*/

	return imports, err
}
