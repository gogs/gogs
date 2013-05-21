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
func GetGithubDoc(client *http.Client, match map[string]string, installGOPATH, commit string, cmdFlags map[string]bool) (*Node, []string, error) {
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
	projectPath := expand("github.com/{owner}/{repo}", match)
	installPath := installGOPATH + "/src/" + projectPath

	// Remove old files.
	os.RemoveAll(installPath + "/")
	// Create destination directory.
	os.MkdirAll(installPath+"/", os.ModePerm)

	dirs := make([]string, 0, 5)
	// Need to add root path because we cannot get from tarball.
	dirs = append(dirs, installPath+"/")
	for _, f := range r.File {
		absPath := strings.Replace(f.FileInfo().Name(), shaName, installPath, 1)

		// Check if it is a directory.
		if strings.HasSuffix(absPath, "/") {
			// Directory.
			// Check if current directory is example.
			if !(!cmdFlags["-e"] && strings.Contains(absPath, "example")) {
				dirs = append(dirs, absPath)
			}
			continue
		}

		// Get file from archive.
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}

		// Create diretory before create file.
		os.MkdirAll(path.Dir(absPath)+"/", os.ModePerm)

		// Write data to file
		fw, _ := os.Create(absPath)
		if err != nil {
			return nil, nil, err
		}

		_, err = io.Copy(fw, rc)
		rc.Close()
		fw.Close()
		if err != nil {
			return nil, nil, err
		}
	}

	node := &Node{
		ImportPath: projectPath,
		Commit:     commit,
	}

	var imports []string

	// Check if need to check imports.
	if isCheckImport {
		for _, d := range dirs {
			importPkgs, err := checkImports(d, match["importPath"])
			if err != nil {
				return nil, nil, err
			}
			imports = append(imports, importPkgs...)
		}
	}

	return node, imports, err
}
