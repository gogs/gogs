// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/GPMGo/gpm/utils"
)

var (
	githubRawHeader = http.Header{"Accept": {"application/vnd.github-blob.raw"}}
	GithubPattern   = regexp.MustCompile(`^github\.com/(?P<owner>[a-z0-9A-Z_.\-]+)/(?P<repo>[a-z0-9A-Z_.\-]+)(?P<dir>/[a-z0-9A-Z_.\-/]*)?$`)
	githubCred      string
)

func SetGithubCredentials(id, secret string) {
	githubCred = "client_id=" + id + "&client_secret=" + secret
}

// GetGithubDoc downloads tarball from github.com.
func GetGithubDoc(client *http.Client, match map[string]string, commit string) (*Package, []string, error) {
	SetGithubCredentials("1862bcb265171f37f36c", "308d71ab53ccd858416cfceaed52d5d5b7d53c5f")
	match["cred"] = githubCred

	// JSON struct for github.com.
	var refs []*struct {
		Ref    string
		Url    string
		Object struct {
			Sha  string
			Type string
			Url  string
		}
	}

	// bundle and snapshot will have commit 'B' and 'S',
	// but does not need to download dependencies.
	isCheckImport := len(commit) == 0

	// Check if download with specific revision.
	if isCheckImport || len(commit) == 1 {
		// Get up-to-date version.
		err := httpGetJSON(client, expand("https://api.github.com/repos/{owner}/{repo}/git/refs?{cred}", match), &refs)
		if err != nil {
			return nil, nil, err
		}

		tags := make(map[string]string)
		for _, ref := range refs {
			switch {
			case strings.HasPrefix(ref.Ref, "refs/heads/"):
				tags[ref.Ref[len("refs/heads/"):]] = ref.Object.Sha
			case strings.HasPrefix(ref.Ref, "refs/tags/"):
				tags[ref.Ref[len("refs/tags/"):]] = ref.Object.Sha
			}
		}

		// Check revision tag.
		match["tag"], commit, err = bestTag(tags, "master")
		if err != nil {
			return nil, nil, err
		}
	}

	// We use .zip here.
	// zip : https://github.com/{owner}/{repo}/archive/{sha}.zip
	// tarball : https://github.com/{owner}/{repo}/tarball/{sha}

	match["sha"] = commit
	// Downlaod archive.
	p, err := httpGetBytes(client, expand("https://github.com/{owner}/{repo}/archive/{sha}.zip", match), nil)
	if err != nil {
		return nil, nil, err
	}

	r, err := zip.NewReader(bytes.NewReader(p), int64(len(p)))
	if err != nil {
		return nil, nil, err
	}

	shaName := expand("{repo}-{sha}", match)
	paths := utils.GetGOPATH()
	importPath := "github.com/" + expand("{owner}/{repo}", match)
	installPath := paths[0] + "/src/" + importPath

	// Remove old files.
	os.RemoveAll(installPath)
	// Create destination directory.
	os.Mkdir(installPath, os.ModePerm)

	dirs := make([]string, 0, 5)
	for _, f := range r.File {
		absPath := strings.Replace(f.FileInfo().Name(), shaName, installPath, 1)

		// Check if it is directory or not.
		if strings.HasSuffix(absPath, "/") {
			// Directory.
			dirs = append(dirs, absPath)
			continue
		}

		// Get files from archive.
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}

		// Create diretory before create file
		os.MkdirAll(path.Dir(absPath), os.ModePerm)
		// Write data to file
		fw, _ := os.Create(absPath)
		if err != nil {
			return nil, nil, err
		}

		_, err = io.Copy(fw, rc)
		if err != nil {
			return nil, nil, err
		}
	}

	pkg := &Package{
		ImportPath: importPath,
		AbsPath:    installPath,
		Commit:     commit,
	}

	var imports []string

	// Check if need to check imports.
	if isCheckImport {
		for _, d := range dirs {
			dir, err := os.Open(d)
			if err != nil {
				return nil, nil, err
			}
			defer dir.Close()

			// Get file info slice.
			fis, err := dir.Readdir(0)
			if err != nil {
				return nil, nil, err
			}

			files := make([]*source, 0, 10)
			for _, fi := range fis {
				// Only handle files.
				if strings.HasSuffix(fi.Name(), ".go") {
					f, err := os.Open(d + "/" + fi.Name())
					if err != nil {
						return nil, nil, err
					}
					defer f.Close()

					fbytes := make([]byte, fi.Size())
					_, err = f.Read(fbytes)
					if err != nil {
						return nil, nil, err
					}

					files = append(files, &source{
						name: importPath + "/" + fi.Name(),
						data: fbytes,
					})
				}
			}

			// Check if has Go source files.
			if len(files) > 0 {
				w := &walker{ImportPath: importPath}
				importPkgs, err := w.build(files)
				if err != nil {
					return nil, nil, err
				}
				imports = append(imports, importPkgs...)
			}
		}
	}

	return pkg, imports, err
}
