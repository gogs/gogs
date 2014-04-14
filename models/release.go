// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/gogits/git"
)

var (
	ErrReleaseAlreadyExist = errors.New("Release already exist")
)

// Release represents a release of repository.
type Release struct {
	Id               int64
	RepoId           int64
	PublisherId      int64
	Publisher        *User `xorm:"-"`
	Title            string
	TagName          string
	LowerTagName     string
	SHA1             string
	NumCommits       int
	NumCommitsBehind int    `xorm:"-"`
	Note             string `xorm:"TEXT"`
	IsPrerelease     bool
	Created          time.Time `xorm:"created"`
}

// GetReleasesByRepoId returns a list of releases of repository.
func GetReleasesByRepoId(repoId int64) (rels []*Release, err error) {
	err = orm.Desc("created").Find(&rels, Release{RepoId: repoId})
	return rels, err
}

// IsReleaseExist returns true if release with given tag name already exists.
func IsReleaseExist(repoId int64, tagName string) (bool, error) {
	if len(tagName) == 0 {
		return false, nil
	}

	return orm.Get(&Release{RepoId: repoId, LowerTagName: strings.ToLower(tagName)})
}

// CreateRelease creates a new release of repository.
func CreateRelease(repoPath string, rel *Release, gitRepo *git.Repository) error {
	isExist, err := IsReleaseExist(rel.RepoId, rel.TagName)
	if err != nil {
		return err
	} else if isExist {
		return ErrReleaseAlreadyExist
	}

	if !git.IsTagExist(repoPath, rel.TagName) {
		_, stderr, err := com.ExecCmdDir(repoPath, "git", "tag", rel.TagName, "-m", rel.Title)
		if err != nil {
			return err
		} else if strings.Contains(stderr, "fatal:") {
			return errors.New(stderr)
		}
	} else {
		commit, err := gitRepo.GetCommitOfTag(rel.TagName)
		if err != nil {
			return err
		}

		rel.NumCommits, err = commit.CommitsCount()
		if err != nil {
			return err
		}
	}

	rel.LowerTagName = strings.ToLower(rel.TagName)
	_, err = orm.InsertOne(rel)
	return err
}
