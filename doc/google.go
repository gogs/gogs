// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"errors"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

var (
	googleRepoRe     = regexp.MustCompile(`id="checkoutcmd">(hg|git|svn)`)
	googleRevisionRe = regexp.MustCompile(`<h2>(?:[^ ]+ - )?Revision *([^:]+):`)
	googleEtagRe     = regexp.MustCompile(`^(hg|git|svn)-`)
	googleFileRe     = regexp.MustCompile(`<li><a href="([^"/]+)"`)
	googleDirRe      = regexp.MustCompile(`<li><a href="([^".]+)"`)
	GooglePattern    = regexp.MustCompile(`^code\.google\.com/p/(?P<repo>[a-z0-9\-]+)(:?\.(?P<subrepo>[a-z0-9\-]+))?(?P<dir>/[a-z0-9A-Z_.\-/]+)?$`)
)

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
	p, err := httpGetBytes(client, expand("http://code.google.com/p/{repo}/source/checkout", match), nil)
	if err != nil {
		return err
	}
	m := googleRepoRe.FindSubmatch(p)
	if m == nil {
		return NotFoundError{"Could not VCS on Google Code project page."}
	}

	match["vcs"] = string(m[1])
	return nil
}

// GetGoogleDoc downloads raw files from code.google.com.
func GetGoogleDoc(client *http.Client, match map[string]string, installGOPATH, commit string, cmdFlags map[string]bool) (*Package, []string, error) {
	setupGoogleMatch(match)
	// Check version control.
	if m := googleEtagRe.FindStringSubmatch(commit); m != nil {
		match["vcs"] = m[1]
	} else if err := getGoogleVCS(client, match); err != nil {
		return nil, nil, err
	}

	// bundle and snapshot will have commit 'B' and 'S',
	// but does not need to download dependencies.
	isCheckImport := len(commit) == 0

	rootPath := expand("http://{subrepo}{dot}{repo}.googlecode.com/{vcs}{dir}/", match)

	// Scrape the repo browser to find the project revision and individual Go files.
	p, err := httpGetBytes(client, rootPath+"?r="+commit, nil)
	if err != nil {
		return nil, nil, err
	}

	// Check revision tag.
	if m := googleRevisionRe.FindSubmatch(p); m == nil {
		return nil, nil,
			errors.New("doc.GetGoogleDoc(): Could not find revision for " + match["importPath"])
	}

	importPath := "code.google.com/p/" + expand("{repo}{dir}", match)
	installPath := installGOPATH + "/src/" + importPath

	// Remove old files.
	os.RemoveAll(installPath + "/")
	// Create destination directory.
	os.MkdirAll(installPath+"/", os.ModePerm)

	// Get source files in root path.
	files := make([]*source, 0, 5)
	for _, m := range googleFileRe.FindAllSubmatch(p, -1) {
		fname := string(m[1])
		files = append(files, &source{
			name:   fname,
			rawURL: expand("http://{subrepo}{dot}{repo}.googlecode.com/{vcs}{dir}/{0}", match, fname) + "?r=" + commit,
		})
	}

	// Fetch files from VCS.
	if err := fetchFiles(client, files, nil); err != nil {
		return nil, nil, err
	}

	// Save files.
	for _, f := range files {
		absPath := installPath + "/"

		// Create diretory before create file.
		os.MkdirAll(path.Dir(absPath), os.ModePerm)

		// Write data to file
		fw, err := os.Create(absPath + f.name)
		if err != nil {
			return nil, nil, err
		}

		_, err = fw.Write(f.data)
		fw.Close()
		if err != nil {
			return nil, nil, err
		}
	}

	dirs := make([]string, 0, 3)
	// Get subdirectories.
	for _, m := range googleDirRe.FindAllSubmatch(p, -1) {
		dirName := strings.Split(string(m[1]), "?")[0]
		if strings.HasSuffix(dirName, "/") {
			dirs = append(dirs, dirName)
		}
	}

	err = downloadFiles(client, match, rootPath, installPath+"/", commit, dirs)
	if err != nil {
		return nil, nil, err
	}

	pkg := &Package{
		ImportPath: importPath,
		AbsPath:    installPath,
		Commit:     commit,
	}
	var imports []string

	// Check if need to check imports.
	if isCheckImport {
		rootdir, err := os.Open(installPath + "/")
		if err != nil {
			return nil, nil, err
		}
		defer rootdir.Close()

		dirs, err := rootdir.Readdir(0)
		if err != nil {
			return nil, nil, err
		}

		for _, d := range dirs {
			if d.IsDir() {
				absPath := installPath + "/" + d.Name() + "/"
				imports, err = checkImports(absPath, importPath)
				if err != nil {
					return nil, nil, err
				}
			}
		}
	}

	return pkg, imports, err
}

func downloadFiles(client *http.Client, match map[string]string, rootPath, installPath, commit string, dirs []string) error {
	for _, d := range dirs {
		p, err := httpGetBytes(client, rootPath+d+"?r="+commit, nil)
		if err != nil {
			return err
		}

		// Create destination directory.
		os.MkdirAll(installPath+d, os.ModePerm)

		// Get source files in current path.
		files := make([]*source, 0, 5)
		for _, m := range googleFileRe.FindAllSubmatch(p, -1) {
			fname := string(m[1])
			files = append(files, &source{
				name:   fname,
				rawURL: expand("http://{subrepo}{dot}{repo}.googlecode.com/{vcs}{dir}/", match) + d + fname + "?r=" + commit,
			})
		}

		// Fetch files from VCS.
		if err := fetchFiles(client, files, nil); err != nil {
			return err
		}

		// Save files.
		for _, f := range files {
			absPath := installPath + d

			// Create diretory before create file.
			os.MkdirAll(path.Dir(absPath), os.ModePerm)

			// Write data to file
			fw, err := os.Create(absPath + f.name)
			if err != nil {
				return err
			}

			_, err = fw.Write(f.data)
			fw.Close()
			if err != nil {
				return err
			}
		}

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
