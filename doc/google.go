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
	googleRepoRe  = regexp.MustCompile(`id="checkoutcmd">(hg|git|svn)`)
	googleFileRe  = regexp.MustCompile(`<li><a href="([^"/]+)"`)
	googleDirRe   = regexp.MustCompile(`<li><a href="([^".]+)"`)
	googlePattern = regexp.MustCompile(`^code\.google\.com/p/(?P<repo>[a-z0-9\-]+)(:?\.(?P<subrepo>[a-z0-9\-]+))?(?P<dir>/[a-z0-9A-Z_.\-/]+)?$`)
)

// getGoogleDoc downloads raw files from code.google.com.
func getGoogleDoc(client *http.Client, match map[string]string, installRepoPath string, nod *Node, cmdFlags map[string]bool) ([]string, error) {
	setupGoogleMatch(match)
	// Check version control.
	if err := getGoogleVCS(client, match); err != nil {
		return nil, errors.New("fail to get vcs " + nod.ImportPath + " : " + err.Error())
	}

	switch nod.Type {
	case BRANCH:
		if len(nod.Value) == 0 {
			match["tag"] = defaultTags[match["vcs"]]
		} else {
			match["tag"] = nod.Value
		}
	case TAG, COMMIT:
		match["tag"] = nod.Value
	default:
		return nil, errors.New("Unknown node type: " + nod.Type)
	}

	var installPath string
	projectPath := GetProjectPath(nod.ImportPath)
	if nod.ImportPath == nod.DownloadURL {
		suf := "." + nod.Value
		if len(suf) == 1 {
			suf = ""
		}
		installPath = installRepoPath + "/" + projectPath + suf
	} else {
		installPath = installRepoPath + "/" + projectPath
	}

	// Remove old files.
	os.RemoveAll(installPath + "/")
	os.MkdirAll(installPath+"/", os.ModePerm)

	if match["vcs"] == "svn" {
		com.ColorLog("[WARN] SVN detected, may take very long time.\n")

		rootPath := com.Expand("http://{subrepo}{dot}{repo}.googlecode.com/{vcs}", match)
		d, f := path.Split(rootPath)
		err := downloadFiles(client, match, d, installPath+"/", match["tag"],
			[]string{f + "/"})
		if err != nil {
			return nil, errors.New("Fail to download " + nod.ImportPath + " : " + err.Error())
		}
	}

	p, err := com.HttpGetBytes(client, com.Expand("http://{subrepo}{dot}{repo}.googlecode.com/archive/{tag}.zip", match), nil)
	if err != nil {
		return nil, errors.New("Fail to download " + nod.ImportPath + " : " + err.Error())
	}

	r, err := zip.NewReader(bytes.NewReader(p), int64(len(p)))
	if err != nil {
		return nil, errors.New(nod.ImportPath + " -> new zip: " + err.Error())
	}

	nameLen := strings.Index(r.File[0].Name, "/")
	dirPrefix := match["dir"]
	if len(dirPrefix) != 0 {
		dirPrefix = dirPrefix[1:] + "/"
	}

	dirs := make([]string, 0, 5)
	for _, f := range r.File {
		absPath := strings.Replace(f.Name, f.Name[:nameLen], installPath, 1)

		// Create diretory before create file.
		dir := path.Dir(absPath)
		if !checkDir(dir, dirs) && !(!cmdFlags["-e"] && strings.Contains(absPath, "example")) {
			dirs = append(dirs, dir)
			os.MkdirAll(dir+"/", os.ModePerm)
		}

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

type rawFile struct {
	name   string
	rawURL string
	data   []byte
}

func (rf *rawFile) Name() string {
	return rf.name
}

func (rf *rawFile) RawUrl() string {
	return rf.rawURL
}

func (rf *rawFile) Data() []byte {
	return rf.data
}

func (rf *rawFile) SetData(p []byte) {
	rf.data = p
}

func downloadFiles(client *http.Client, match map[string]string, rootPath, installPath, commit string, dirs []string) error {
	suf := "?r=" + commit
	if len(commit) == 0 {
		suf = ""
	}

	for _, d := range dirs {
		p, err := com.HttpGetBytes(client, rootPath+d+suf, nil)
		if err != nil {
			return err
		}

		// Create destination directory.
		os.MkdirAll(installPath+d, os.ModePerm)

		// Get source files in current path.
		files := make([]com.RawFile, 0, 5)
		for _, m := range googleFileRe.FindAllSubmatch(p, -1) {
			fname := strings.Split(string(m[1]), "?")[0]
			files = append(files, &rawFile{
				name:   fname,
				rawURL: rootPath + d + fname + suf,
			})
		}

		// Fetch files from VCS.
		if err := com.FetchFilesCurl(files); err != nil {
			return err
		}

		// Save files.
		for _, f := range files {
			absPath := installPath + d

			// Create diretory before create file.
			os.MkdirAll(path.Dir(absPath), os.ModePerm)

			// Write data to file
			fw, err := os.Create(absPath + f.Name())
			if err != nil {
				return err
			}

			_, err = fw.Write(f.Data())
			fw.Close()
			if err != nil {
				return err
			}
		}
		files = nil

		subdirs := make([]string, 0, 3)
		// Get subdirectories.
		for _, m := range googleDirRe.FindAllSubmatch(p, -1) {
			dirName := strings.Split(string(m[1]), "?")[0]
			if strings.HasSuffix(dirName, "/") {
				subdirs = append(subdirs, d+dirName)
			}
		}

		err = downloadFiles(client, match, rootPath, installPath, commit, subdirs)
		if err != nil {
			return err
		}
	}
	return nil
}

func setupGoogleMatch(match map[string]string) {
	if s := match["subrepo"]; s != "" {
		match["dot"] = "."
		match["query"] = "?repo=" + s
	} else {
		match["dot"] = ""
		match["query"] = ""
	}
}

func getGoogleVCS(client *http.Client, match map[string]string) error {
	// Scrape the HTML project page to find the VCS.
	p, err := com.HttpGetBytes(client, com.Expand("http://code.google.com/p/{repo}/source/checkout", match), nil)
	if err != nil {
		return errors.New("doc.getGoogleVCS(" + match["importPath"] + ") -> " + err.Error())
	}
	m := googleRepoRe.FindSubmatch(p)
	if m == nil {
		return com.NotFoundError{"Could not VCS on Google Code project page."}
	}
	match["vcs"] = string(m[1])
	return nil
}
