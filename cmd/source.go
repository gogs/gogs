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
	downloadCache map[string]bool
	sources       []Source = []Source{
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
