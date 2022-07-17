// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repoutil

import (
	"fmt"
	"path/filepath"
	"strings"

	"gogs.io/gogs/internal/conf"
)

// CloneLink represents different types of clone URLs of repository.
type CloneLink struct {
	SSH   string
	HTTPS string
}

// NewCloneLink returns clone URLs using given owner and repository name.
func NewCloneLink(owner, repo string, isWiki bool) *CloneLink {
	if isWiki {
		repo += ".wiki"
	}

	cl := new(CloneLink)
	if conf.SSH.Port != 22 {
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", conf.App.RunUser, conf.SSH.Domain, conf.SSH.Port, owner, repo)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", conf.App.RunUser, conf.SSH.Domain, owner, repo)
	}
	cl.HTTPS = HTTPSCloneURL(owner, repo)
	return cl
}

// HTTPSCloneURL returns HTTPS clone URL using given owner and repository name.
func HTTPSCloneURL(owner, repo string) string {
	return fmt.Sprintf("%s%s/%s.git", conf.Server.ExternalURL, owner, repo)
}

// HTMLURL returns HTML URL using given owner and repository name.
func HTMLURL(owner, repo string) string {
	return conf.Server.ExternalURL + owner + "/" + repo
}

// CompareCommitsPath returns the comparison path using given owner, repository,
// and commit IDs.
func CompareCommitsPath(owner, repo, oldCommitID, newCommitID string) string {
	return fmt.Sprintf("%s/%s/compare/%s...%s", owner, repo, oldCommitID, newCommitID)
}

// UserPath returns the absolute path for storing user repositories.
func UserPath(user string) string {
	return filepath.Join(conf.Repository.Root, strings.ToLower(user))
}

// RepositoryPath returns the absolute path using given user and repository
// name.
func RepositoryPath(owner, repo string) string {
	return filepath.Join(UserPath(owner), strings.ToLower(repo)+".git")
}
