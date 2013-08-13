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
	PkgUrl(pkgName string, ver string) string
	HasPkg(pkgName string) bool
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

func NewPkg(pkgName string, ver string) *Pkg {
	vers := strings.Split(ver, ":")
	if len(vers) > 2 {
		return nil
	}

	var verId string
	if len(vers) == 2 {
		verId = vers[1]
	}

	return &Pkg{
		getSource(pkgName), pkgName, vers[0], verId,
	}
}

type GithubSource struct {
}

func (s *GithubSource) PkgUrl(pkgName string, ver string) string {
	vers := strings.Split(ver, ":")
	var verPath string
	switch strings.ToLower(vers[0]) {
	case TRUNK:
		verPath = "master"
	case TAG, COMMIT, BRANCH:
		if len(vers) != 2 {
			return ""
		}
		verPath = vers[1]
	default:
		return ""
	}
	return fmt.Sprintf("https://%v/archive/%v.zip", pkgName, verPath)
}

func (s *GithubSource) HasPkg(pkgName string) bool {
	return strings.HasPrefix(pkgName, "github.com")
}

type GitLabSource struct {
}
