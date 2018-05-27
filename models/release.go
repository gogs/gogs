// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/process"
)

// Release represents a release of repository.
type Release struct {
	ID               int64
	RepoID           int64
	Repo             *Repository `xorm:"-"`
	PublisherID      int64
	Publisher        *User `xorm:"-"`
	TagName          string
	LowerTagName     string
	Target           string
	Title            string
	Sha1             string `xorm:"VARCHAR(40)"`
	NumCommits       int64
	NumCommitsBehind int64  `xorm:"-"`
	Note             string `xorm:"TEXT"`
	IsDraft          bool   `xorm:"NOT NULL DEFAULT false"`
	IsPrerelease     bool

	Created     time.Time `xorm:"-"`
	CreatedUnix int64

	Attachments []*Attachment `xorm:"-"`
}

func (r *Release) BeforeInsert() {
	if r.CreatedUnix == 0 {
		r.CreatedUnix = time.Now().Unix()
	}
}

func (r *Release) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		r.Created = time.Unix(r.CreatedUnix, 0).Local()
	}
}

func (r *Release) loadAttributes(e Engine) (err error) {
	if r.Repo == nil {
		r.Repo, err = getRepositoryByID(e, r.RepoID)
		if err != nil {
			return fmt.Errorf("getRepositoryByID [repo_id: %d]: %v", r.RepoID, err)
		}
	}

	if r.Publisher == nil {
		r.Publisher, err = getUserByID(e, r.PublisherID)
		if err != nil {
			if errors.IsUserNotExist(err) {
				r.PublisherID = -1
				r.Publisher = NewGhostUser()
			} else {
				return fmt.Errorf("getUserByID.(Publisher) [publisher_id: %d]: %v", r.PublisherID, err)
			}
		}
	}

	if r.Attachments == nil {
		r.Attachments, err = getAttachmentsByReleaseID(e, r.ID)
		if err != nil {
			return fmt.Errorf("getAttachmentsByReleaseID [%d]: %v", r.ID, err)
		}
	}

	return nil
}

func (r *Release) LoadAttributes() error {
	return r.loadAttributes(x)
}

// This method assumes some fields assigned with values:
// Required - Publisher
func (r *Release) APIFormat() *api.Release {
	return &api.Release{
		ID:              r.ID,
		TagName:         r.TagName,
		TargetCommitish: r.Target,
		Name:            r.Title,
		Body:            r.Note,
		Draft:           r.IsDraft,
		Prerelease:      r.IsPrerelease,
		Author:          r.Publisher.APIFormat(),
		Created:         r.Created,
	}
}

// IsReleaseExist returns true if release with given tag name already exists.
func IsReleaseExist(repoID int64, tagName string) (bool, error) {
	if len(tagName) == 0 {
		return false, nil
	}

	return x.Get(&Release{RepoID: repoID, LowerTagName: strings.ToLower(tagName)})
}

func createTag(gitRepo *git.Repository, r *Release) error {
	// Only actual create when publish.
	if !r.IsDraft {
		if !gitRepo.IsTagExist(r.TagName) {
			commit, err := gitRepo.GetBranchCommit(r.Target)
			if err != nil {
				return fmt.Errorf("GetBranchCommit: %v", err)
			}

			// Trim '--' prefix to prevent command line argument vulnerability.
			r.TagName = strings.TrimPrefix(r.TagName, "--")
			if err = gitRepo.CreateTag(r.TagName, commit.ID.String()); err != nil {
				if strings.Contains(err.Error(), "is not a valid tag name") {
					return ErrInvalidTagName{r.TagName}
				}
				return err
			}
		} else {
			commit, err := gitRepo.GetTagCommit(r.TagName)
			if err != nil {
				return fmt.Errorf("GetTagCommit: %v", err)
			}

			r.Sha1 = commit.ID.String()
			r.NumCommits, err = commit.CommitsCount()
			if err != nil {
				return fmt.Errorf("CommitsCount: %v", err)
			}
		}
	}
	return nil
}

func (r *Release) preparePublishWebhooks() {
	if err := PrepareWebhooks(r.Repo, HOOK_EVENT_RELEASE, &api.ReleasePayload{
		Action:     api.HOOK_RELEASE_PUBLISHED,
		Release:    r.APIFormat(),
		Repository: r.Repo.APIFormat(nil),
		Sender:     r.Publisher.APIFormat(),
	}); err != nil {
		log.Error(2, "PrepareWebhooks: %v", err)
	}
}

// NewRelease creates a new release with attachments for repository.
func NewRelease(gitRepo *git.Repository, r *Release, uuids []string) error {
	isExist, err := IsReleaseExist(r.RepoID, r.TagName)
	if err != nil {
		return err
	} else if isExist {
		return ErrReleaseAlreadyExist{r.TagName}
	}

	if err = createTag(gitRepo, r); err != nil {
		return err
	}
	r.LowerTagName = strings.ToLower(r.TagName)

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(r); err != nil {
		return fmt.Errorf("Insert: %v", err)
	}

	if len(uuids) > 0 {
		if _, err = sess.In("uuid", uuids).Cols("release_id").Update(&Attachment{ReleaseID: r.ID}); err != nil {
			return fmt.Errorf("link attachments: %v", err)
		}
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	// Only send webhook when actually published, skip drafts
	if r.IsDraft {
		return nil
	}
	r, err = GetReleaseByID(r.ID)
	if err != nil {
		return fmt.Errorf("GetReleaseByID: %v", err)
	}
	r.preparePublishWebhooks()
	return nil
}

// GetRelease returns release by given ID.
func GetRelease(repoID int64, tagName string) (*Release, error) {
	isExist, err := IsReleaseExist(repoID, tagName)
	if err != nil {
		return nil, err
	} else if !isExist {
		return nil, ErrReleaseNotExist{0, tagName}
	}

	r := &Release{RepoID: repoID, LowerTagName: strings.ToLower(tagName)}
	if _, err = x.Get(r); err != nil {
		return nil, fmt.Errorf("Get: %v", err)
	}

	return r, r.LoadAttributes()
}

// GetReleaseByID returns release with given ID.
func GetReleaseByID(id int64) (*Release, error) {
	r := new(Release)
	has, err := x.Id(id).Get(r)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrReleaseNotExist{id, ""}
	}

	return r, r.LoadAttributes()
}

// GetPublishedReleasesByRepoID returns a list of published releases of repository.
// If matches is not empty, only published releases in matches will be returned.
// In any case, drafts won't be returned by this function.
func GetPublishedReleasesByRepoID(repoID int64, matches ...string) ([]*Release, error) {
	sess := x.Where("repo_id = ?", repoID).And("is_draft = ?", false).Desc("created_unix")
	if len(matches) > 0 {
		sess.In("tag_name", matches)
	}
	releases := make([]*Release, 0, 5)
	return releases, sess.Find(&releases, new(Release))
}

// GetDraftReleasesByRepoID returns all draft releases of repository.
func GetDraftReleasesByRepoID(repoID int64) ([]*Release, error) {
	releases := make([]*Release, 0)
	return releases, x.Where("repo_id = ?", repoID).And("is_draft = ?", true).Find(&releases)
}

type ReleaseSorter struct {
	releases []*Release
}

func (rs *ReleaseSorter) Len() int {
	return len(rs.releases)
}

func (rs *ReleaseSorter) Less(i, j int) bool {
	diffNum := rs.releases[i].NumCommits - rs.releases[j].NumCommits
	if diffNum != 0 {
		return diffNum > 0
	}
	return rs.releases[i].Created.After(rs.releases[j].Created)
}

func (rs *ReleaseSorter) Swap(i, j int) {
	rs.releases[i], rs.releases[j] = rs.releases[j], rs.releases[i]
}

// SortReleases sorts releases by number of commits and created time.
func SortReleases(rels []*Release) {
	sorter := &ReleaseSorter{releases: rels}
	sort.Sort(sorter)
}

// UpdateRelease updates information of a release.
func UpdateRelease(doer *User, gitRepo *git.Repository, r *Release, isPublish bool, uuids []string) (err error) {
	if err = createTag(gitRepo, r); err != nil {
		return fmt.Errorf("createTag: %v", err)
	}

	r.PublisherID = doer.ID

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}
	if _, err = sess.Id(r.ID).AllCols().Update(r); err != nil {
		return fmt.Errorf("Update: %v", err)
	}

	// Unlink all current attachments and link back later if still valid
	if _, err = sess.Exec("UPDATE attachment SET release_id = 0 WHERE release_id = ?", r.ID); err != nil {
		return fmt.Errorf("unlink current attachments: %v", err)
	}

	if len(uuids) > 0 {
		if _, err = sess.In("uuid", uuids).Cols("release_id").Update(&Attachment{ReleaseID: r.ID}); err != nil {
			return fmt.Errorf("link attachments: %v", err)
		}
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	if !isPublish {
		return nil
	}
	r.Publisher = doer
	r.preparePublishWebhooks()
	return nil
}

// DeleteReleaseOfRepoByID deletes a release and corresponding Git tag by given ID.
func DeleteReleaseOfRepoByID(repoID, id int64) error {
	rel, err := GetReleaseByID(id)
	if err != nil {
		return fmt.Errorf("GetReleaseByID: %v", err)
	}

	// Mark sure the delete operation againsts same repository.
	if repoID != rel.RepoID {
		return nil
	}

	repo, err := GetRepositoryByID(rel.RepoID)
	if err != nil {
		return fmt.Errorf("GetRepositoryByID: %v", err)
	}

	_, stderr, err := process.ExecDir(-1, repo.RepoPath(),
		fmt.Sprintf("DeleteReleaseByID (git tag -d): %d", rel.ID),
		"git", "tag", "-d", rel.TagName)
	if err != nil && !strings.Contains(stderr, "not found") {
		return fmt.Errorf("git tag -d: %v - %s", err, stderr)
	}

	if _, err = x.Id(rel.ID).Delete(new(Release)); err != nil {
		return fmt.Errorf("Delete: %v", err)
	}

	return nil
}
