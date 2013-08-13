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

package cmd

import (
	//"errors"
	"fmt"
	"strings"
)

const (
	TRUNK  = "trunk"
	TAG    = "tag"
	BRANCH = "branch"
	COMMIT = "commit"
)

var (
	downloadCache map[string]bool // Saves packages that have been downloaded.
	sources       []Source        = []Source{
		&GithubSource{},
		&GitOscSource{},
	}
)

func getSource(pkgName string) Source {
	for _, source := range sources {
		if source.HasPkg(pkgName) {
			return source
		}
	}
	return nil
}

type Source interface {
	PkgUrl(pkg *Pkg) string
	HasPkg(pkgName string) bool
	PkgExt() string
}

type Pkg struct {
	Source Source
	Name   string
	Ver    string
	VerId  string
}

func (p *Pkg) VerSimpleString() string {
	if p.VerId != "" {
		return p.VerId
	}
	return p.Ver
}

func (p *Pkg) VerString() string {
	if p.VerId == "" {
		return p.Ver
	}
	return fmt.Sprintf("%v:%v", p.Ver, p.VerId)
}

func (p *Pkg) Url() string {
	return p.Source.PkgUrl(p)
}

func (p *Pkg) FileName() string {
	return fmt.Sprintf("%v.%v", p.VerSimpleString(), p.Source.PkgExt())
}

func NewPkg(pkgName string, ver string) *Pkg {
	vers := strings.Split(ver, ":")
	if len(vers) > 2 {
		return nil
	}

	var verId string
	if len(vers) == 2 {
		verId = vers[1]
	}

	source := getSource(pkgName)
	if source == nil {
		return nil
	}

	return &Pkg{source, pkgName, vers[0], verId}
}

// github repository
type GithubSource struct {
}

func (s *GithubSource) PkgUrl(pkg *Pkg) string {
	var verPath string
	if pkg.Ver == TRUNK {
		verPath = "master"
	} else {
		verPath = pkg.VerId
	}
	return fmt.Sprintf("https://%v/archive/%v.zip", pkg.Name, verPath)
}

func (s *GithubSource) HasPkg(pkgName string) bool {
	return strings.HasPrefix(pkgName, "github.com")
}

func (s *GithubSource) PkgExt() string {
	return "zip"
}

// git osc repos
type GitOscSource struct {
}

func (s *GitOscSource) PkgUrl(pkg *Pkg) string {
	var verPath string
	if pkg.Ver == TRUNK {
		verPath = "master"
	} else {
		verPath = pkg.VerId
	}
	return fmt.Sprintf("https://%v/repository/archive?ref=%v", pkg.Name, verPath)
}

func (s *GitOscSource) HasPkg(pkgName string) bool {
	return strings.HasPrefix(pkgName, "git.oschina.net")
}

func (s *GitOscSource) PkgExt() string {
	return "zip"
}

type GitLabSource struct {
	IP         string
	Username   string
	Passwd     string
	PrivateKey string
}
