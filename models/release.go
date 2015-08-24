// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/git"
)

var (
	ErrReleaseAlreadyExist = errors.New("Release already exist")
	ErrReleaseNotExist     = errors.New("Release does not exist")
)

// Release represents a release of repository.
type Release struct {
	Id               int64
	RepoId           int64
	PublisherId      int64
	Publisher        *User `xorm:"-"`
	TagName          string
	LowerTagName     string
	Target           string
	Title            string
	Sha1             string `xorm:"VARCHAR(40)"`
	NumCommits       int
	NumCommitsBehind int    `xorm:"-"`
	Note             string `xorm:"TEXT"`
	IsDraft          bool   `xorm:"NOT NULL DEFAULT false"`
	IsPrerelease     bool
	Created          time.Time `xorm:"CREATED"`
}

func (r *Release) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created":
		r.Created = regulateTimeZone(r.Created)
	}
}

// IsReleaseExist returns true if release with given tag name already exists.
func IsReleaseExist(repoId int64, tagName string) (bool, error) {
	if len(tagName) == 0 {
		return false, nil
	}

	return x.Get(&Release{RepoId: repoId, LowerTagName: strings.ToLower(tagName)})
}

func createTag(gitRepo *git.Repository, rel *Release) error {
	// Only actual create when publish.
	if !rel.IsDraft {
		if !gitRepo.IsTagExist(rel.TagName) {
			commit, err := gitRepo.GetCommitOfBranch(rel.Target)
			if err != nil {
				return err
			}

			if err = gitRepo.CreateTag(rel.TagName, commit.Id.String()); err != nil {
				return err
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
	}
	return nil
}

// CreateRelease creates a new release of repository.
func CreateRelease(gitRepo *git.Repository, rel *Release) error {
	isExist, err := IsReleaseExist(rel.RepoId, rel.TagName)
	if err != nil {
		return err
	} else if isExist {
		return ErrReleaseAlreadyExist
	}

	if err = createTag(gitRepo, rel); err != nil {
		return err
	}
	rel.LowerTagName = strings.ToLower(rel.TagName)
	_, err = x.InsertOne(rel)
	return err
}

// GetRelease returns release by given ID.
func GetRelease(repoId int64, tagName string) (*Release, error) {
	isExist, err := IsReleaseExist(repoId, tagName)
	if err != nil {
		return nil, err
	} else if !isExist {
		return nil, ErrReleaseNotExist
	}

	rel := &Release{RepoId: repoId, LowerTagName: strings.ToLower(tagName)}
	_, err = x.Get(rel)
	return rel, err
}

// GetReleasesByRepoId returns a list of releases of repository.
func GetReleasesByRepoId(repoId int64) (rels []*Release, err error) {
	err = x.Desc("created").Find(&rels, Release{RepoId: repoId})
	return rels, err
}

type ReleaseSorter struct {
	rels []*Release
}

func (rs *ReleaseSorter) Len() int {
	return len(rs.rels)
}

func (rs *ReleaseSorter) Less(i, j int) bool {
	diffNum := rs.rels[i].NumCommits - rs.rels[j].NumCommits
	if diffNum != 0 {
		return diffNum > 0
	}
	return rs.rels[i].Created.After(rs.rels[j].Created)
}

func (rs *ReleaseSorter) Swap(i, j int) {
	rs.rels[i], rs.rels[j] = rs.rels[j], rs.rels[i]
}

// SortReleases sorts releases by number of commits and created time.
func SortReleases(rels []*Release) {
	sorter := &ReleaseSorter{rels: rels}
	sort.Sort(sorter)
}

// UpdateRelease updates information of a release.
func UpdateRelease(gitRepo *git.Repository, rel *Release) (err error) {
	if err = createTag(gitRepo, rel); err != nil {
		return err
	}
	_, err = x.Id(rel.Id).AllCols().Update(rel)
	return err
}
