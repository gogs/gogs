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
	"fmt"
	"go/token"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/codegangsta/cli"
)

const (
	TRUNK   = "trunk"
	MASTER  = "master"
	DEFAULT = "default"
	TAG     = "tag"
	BRANCH  = "branch"
	COMMIT  = "commit"
	LOCAL   = "local"
)

type Pkg struct {
	ImportPath string
	RootPath   string
	Type       string
	Value      string // Branch, tag, commit or local.
}

// If the package is fixed and no need to updated.
// For commit, tag and local, it's fixed. For branch
func (pkg *Pkg) IsFixed() bool {
	if pkg.Type == BRANCH || len(pkg.Value) == 0 {
		return false
	}
	return true
}

func (pkg *Pkg) VerString() string {
	if pkg.Value == "" {
		return pkg.Type
	}
	return fmt.Sprintf("%v:%v", pkg.Type, pkg.Value)
}

func NewPkg(importPath, tp, value string) *Pkg {
	return &Pkg{importPath, "", tp, value}
}

func NewDefaultPkg(importPath string) *Pkg {
	return NewPkg(importPath, BRANCH, "")
}

type Node struct {
	Pkg
	DownloadURL   string
	Synopsis      string
	IsGetDeps     bool
	IsGetDepsOnly bool
	Revision      string
}

func NewNode(importPath, downloadUrl, tp, value string, isGetDeps bool) *Node {
	return &Node{
		Pkg: Pkg{
			ImportPath: importPath,
			Type:       tp,
			Value:      value,
		},
		DownloadURL: downloadUrl,
		IsGetDeps:   isGetDeps,
	}
}

// source is source code file.
type source struct {
	rawURL string
	name   string
	data   []byte
}

func (s *source) Name() string       { return s.name }
func (s *source) Size() int64        { return int64(len(s.data)) }
func (s *source) Mode() os.FileMode  { return 0 }
func (s *source) ModTime() time.Time { return time.Time{} }
func (s *source) IsDir() bool        { return false }
func (s *source) Sys() interface{}   { return nil }

// walker holds the state used when building the documentation.
type walker struct {
	ImportPath string
	srcs       map[string]*source // Source files.
	fset       *token.FileSet
}

// service represents a source code control service.
type service struct {
	pattern *regexp.Regexp
	prefix  string
	get     func(*http.Client, map[string]string, string, *Node, *cli.Context) ([]string, error)
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	{githubPattern, "github.com/", getGithubDoc},
	{googlePattern, "code.google.com/", getGoogleDoc},
	{bitbucketPattern, "bitbucket.org/", getBitbucketDoc},
	{launchpadPattern, "launchpad.net/", getLaunchpadDoc},
	{oscPattern, "git.oschina.net/", getOSCDoc},
}
