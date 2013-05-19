// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/GPMGo/gpm/models"
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

func GetGithubDoc(client *http.Client, match map[string]string, commit string) (*models.PkgInfo, error) {
	SetGithubCredentials("1862bcb265171f37f36c", "308d71ab53ccd858416cfceaed52d5d5b7d53c5f")
	match["cred"] = githubCred

	var refs []*struct {
		Object struct {
			Type string
			Sha  string
			Url  string
		}
		Ref string
		Url string
	}

	// Check if has specific commit.
	if len(commit) == 0 {
		// Get up-to-date version.
		err := httpGetJSON(client, expand("https://api.github.com/repos/{owner}/{repo}/git/refs?{cred}", match), &refs)
		if err != nil {
			return nil, err
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
			return nil, err
		}
	}

	match["sha"] = commit
	// Download zip.
	p, err := httpGetBytes(client, expand("https://github.com/{owner}/{repo}/archive/{sha}.zip", match), nil)
	if err != nil {
		return nil, err
	}

	r, err := zip.NewReader(bytes.NewReader(p), int64(len(p)))
	if err != nil {
		return nil, err
	}
	//defer r.Close()

	shaName := expand("{repo}-{sha}", match)
	paths := utils.GetGOPATH()
	importPath := "github.com/" + expand("{owner}/{repo}", match)
	installPath := paths[0] + "/src/" + importPath
	// Create destination directory
	os.Mkdir(installPath, os.ModePerm)

	files := make([]*source, 0, len(r.File))
	for _, f := range r.File {
		srcName := f.FileInfo().Name()[strings.Index(f.FileInfo().Name(), "/")+1:]
		fmt.Printf("Unzipping %s...", srcName)
		fn := strings.Replace(f.FileInfo().Name(), shaName, installPath, 1)

		// Get files from archive
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}

		// Create diretory before create file
		os.MkdirAll(path.Dir(fn), os.ModePerm)
		// Write data to file
		fw, _ := os.Create(fn)
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(fw, rc)
		if err != nil {
			return nil, err
		}

		localF, _ := os.Open(fn)
		fbytes := make([]byte, f.FileInfo().Size())
		n, _ := localF.Read(fbytes)
		fmt.Println(n)

		// Check if Go source file.
		if n > 0 && strings.HasSuffix(fn, ".go") {
			files = append(files, &source{
				name: srcName,
				data: fbytes,
			})
		}
	}

	w := &walker{
		pinfo: &models.PkgInfo{
			Path:   importPath,
			Commit: commit,
		},
	}

	return w.build(files)
}
