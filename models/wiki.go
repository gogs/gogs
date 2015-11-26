// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/git-shell"
)

// ToWikiPageName formats a string to corresponding wiki URL name.
func ToWikiPageName(name string) string {
	return strings.Replace(name, " ", "-", -1)
}

// WikiPath returns wiki data path by given user and repository name.
func WikiPath(userName, repoName string) string {
	return filepath.Join(UserPath(userName), strings.ToLower(repoName)+".wiki.git")
}

func (repo *Repository) WikiPath() string {
	return WikiPath(repo.MustOwner().Name, repo.Name)
}

// HasWiki returns true if repository has wiki.
func (repo *Repository) HasWiki() bool {
	return com.IsDir(repo.WikiPath())
}

// InitWiki initializes a wiki for repository,
// it does nothing when repository already has wiki.
func (repo *Repository) InitWiki() error {
	if repo.HasWiki() {
		return nil
	}

	if err := git.InitRepository(repo.WikiPath(), true); err != nil {
		return fmt.Errorf("InitRepository: %v", err)
	}
	return nil
}

// AddWikiPage adds new page to repository wiki.
func (repo *Repository) AddWikiPage(title, content, message string) (err error) {
	if err = repo.InitWiki(); err != nil {
		return fmt.Errorf("InitWiki: %v", err)
	}

	return nil
}
