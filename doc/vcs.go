// Copyright 2013-2014 gopm authors.
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
	"encoding/xml"
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
	appPath    string
	autoBackup bool
)

func SetAppConfig(path string, backup bool) {
	appPath = path
	autoBackup = backup
}

// TODO: specify with command line flag
const repoRoot = "/tmp/gddo"

var urlTemplates = []struct {
	re       *regexp.Regexp
	template string
	lineFmt  string
}{
	{
		regexp.MustCompile(`^git\.gitorious\.org/(?P<repo>[^/]+/[^/]+)$`),
		"https://gitorious.org/{repo}/blobs/{tag}/{dir}{0}",
		"#line%d",
	},
	{
		regexp.MustCompile(`^camlistore\.org/r/p/(?P<repo>[^/]+)$`),
		"http://camlistore.org/code/?p={repo}.git;hb={tag};f={dir}{0}",
		"#l%d",
	},
}

// lookupURLTemplate finds an expand() template, match map and line number
// format for well known repositories.
func lookupURLTemplate(repo, dir, tag string) (string, map[string]string, string) {
	if strings.HasPrefix(dir, "/") {
		dir = dir[1:] + "/"
	}
	for _, t := range urlTemplates {
		if m := t.re.FindStringSubmatch(repo); m != nil {
			match := map[string]string{
				"dir": dir,
				"tag": tag,
			}
			for i, name := range t.re.SubexpNames() {
				if name != "" {
					match[name] = m[i]
				}
			}
			return t.template, match, t.lineFmt
		}
	}
	return "", nil, ""
}

type vcsCmd struct {
	schemes  []string
	download func([]string, string, string) (string, string, error)
}

var lsremoteRe = regexp.MustCompile(`(?m)^([0-9a-f]{40})\s+refs/(?:tags|heads)/(.+)$`)

var defaultTags = map[string]string{"git": "master", "hg": "default"}

func bestTag(tags map[string]string, defaultTag string) (string, string, error) {
	if commit, ok := tags[defaultTag]; ok {
		return defaultTag, commit, nil
	}
	return "", "", com.NotFoundError{"Tag or branch not found."}
}

// PureDownload downloads package without version control.
func PureDownload(nod *Node, installRepoPath string, ctx *cli.Context) ([]string, error) {
	for _, s := range services {
		if s.get == nil || !strings.HasPrefix(nod.DownloadURL, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(nod.DownloadURL)
		if m == nil {
			if s.prefix != "" {
				return nil, errors.New("Cannot match package service prefix by given path")
			}
			continue
		}
		match := map[string]string{"importPath": nod.DownloadURL}
		for i, n := range s.pattern.SubexpNames() {
			if n != "" {
				match[n] = m[i]
			}
		}
		return s.get(HttpClient, match, installRepoPath, nod, ctx)
	}

	log.Log("Cannot match any service, getting dynamic...")
	return getDynamic(HttpClient, nod, installRepoPath, ctx)
}

func getDynamic(client *http.Client, nod *Node, installRepoPath string, ctx *cli.Context) ([]string, error) {
	match, err := fetchMeta(client, nod.ImportPath)
	if err != nil {
		return nil, err
	}

	if match["projectRoot"] != nod.ImportPath {
		rootMatch, err := fetchMeta(client, match["projectRoot"])
		if err != nil {
			return nil, err
		}
		if rootMatch["projectRoot"] != match["projectRoot"] {
			return nil, com.NotFoundError{"Project root mismatch."}
		}
	}

	nod.DownloadURL = com.Expand("{repo}{dir}", match)
	return PureDownload(nod, installRepoPath, ctx)
}

func fetchMeta(client *http.Client, importPath string) (map[string]string, error) {
	uri := importPath
	if !strings.Contains(uri, "/") {
		// Add slash for root of domain.
		uri = uri + "/"
	}
	uri = uri + "?go-get=1"

	scheme := "https"
	resp, err := client.Get(scheme + "://" + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		scheme = "http"
		resp, err = client.Get(scheme + "://" + uri)
		if err != nil {
			return nil, &com.RemoteError{strings.SplitN(importPath, "/", 2)[0], err}
		}
	}
	defer resp.Body.Close()
	return parseMeta(scheme, importPath, resp.Body)
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

func parseMeta(scheme, importPath string, r io.Reader) (map[string]string, error) {
	var match map[string]string

	d := xml.NewDecoder(r)
	d.Strict = false
metaScan:
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			break metaScan
		}
		switch t := t.(type) {
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "head") {
				break metaScan
			}
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "body") {
				break metaScan
			}
			if !strings.EqualFold(t.Name.Local, "meta") ||
				attrValue(t.Attr, "name") != "go-import" {
				continue metaScan
			}
			f := strings.Fields(attrValue(t.Attr, "content"))
			if len(f) != 3 ||
				!strings.HasPrefix(importPath, f[0]) ||
				!(len(importPath) == len(f[0]) || importPath[len(f[0])] == '/') {
				continue metaScan
			}
			if match != nil {
				return nil, com.NotFoundError{"More than one <meta> found at " + scheme + "://" + importPath}
			}

			projectRoot, vcs, repo := f[0], f[1], f[2]

			repo = strings.TrimSuffix(repo, "."+vcs)
			i := strings.Index(repo, "://")
			if i < 0 {
				return nil, com.NotFoundError{"Bad repo URL in <meta>."}
			}
			proto := repo[:i]
			repo = repo[i+len("://"):]

			match = map[string]string{
				// Used in getVCSDoc, same as vcsPattern matches.
				"importPath": importPath,
				"repo":       repo,
				"vcs":        vcs,
				"dir":        importPath[len(projectRoot):],

				// Used in getVCSDoc
				"scheme": proto,

				// Used in getDynamic.
				"projectRoot": projectRoot,
				"projectName": path.Base(projectRoot),
				"projectURL":  scheme + "://" + projectRoot,
			}
		}
	}
	if match == nil {
		return nil, com.NotFoundError{"<meta> not found."}
	}
	return match, nil
}

func getImports(rootPath string, match map[string]string, cmdFlags map[string]bool, nod *Node) (imports []string) {
	dirs, err := GetDirsInfo(rootPath)
	if err != nil {
		log.Error("", "Fail to get directory's information")
		log.Fatal("", err.Error())
	}

	for _, d := range dirs {
		if d.IsDir() && !(!cmdFlags["-e"] && strings.Contains(d.Name(), "example")) {
			absPath := rootPath + d.Name() + "/"
			importPkgs, err := CheckImports(absPath, match["importPath"], nod)
			if err != nil {
				return nil
			}
			imports = append(imports, importPkgs...)
		}
	}
	return imports
}

// checkImports checks package denpendencies.
func CheckImports(absPath, importPath string, nod *Node) (importPkgs []string, err error) {
	dir, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	// Get file info slice.
	fis, err := dir.Readdir(0)
	if err != nil {
		return nil, err
	}

	files := make([]*source, 0, 10)
	for _, fi := range fis {
		// Only handle files.
		if strings.HasSuffix(fi.Name(), ".go") {
			data, err := com.ReadFile(absPath + fi.Name())
			if err != nil {
				return nil, err
			}

			files = append(files, &source{
				name: fi.Name(),
				data: data,
			})
		}
	}

	// Check if has Go source files.
	if len(files) > 0 {
		w := &walker{ImportPath: importPath}
		importPkgs, err = w.build(files, nod)
		if err != nil {
			return nil, err
		}
	}

	return importPkgs, err
}
