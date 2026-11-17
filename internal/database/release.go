package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// Release represents a release of repository.
type Release struct {
	ID               int64
	RepoID           int64
	Repo             *Repository `xorm:"-" json:"-" gorm:"-"`
	PublisherID      int64
	Publisher        *User `xorm:"-" json:"-" gorm:"-"`
	TagName          string
	LowerTagName     string
	Target           string
	Title            string
	Sha1             string `xorm:"VARCHAR(64)"`
	NumCommits       int64
	NumCommitsBehind int64  `xorm:"-" json:"-" gorm:"-"`
	Note             string `xorm:"TEXT"`
	IsDraft          bool   `xorm:"NOT NULL DEFAULT false"`
	IsPrerelease     bool

	Created     time.Time `xorm:"-" json:"-" gorm:"-"`
	CreatedUnix int64

	Attachments []*Attachment `xorm:"-" json:"-" gorm:"-"`
}

// BeforeCreate implements the GORM create hook.
func (r *Release) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedUnix == 0 {
		r.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (r *Release) AfterFind(_ *gorm.DB) error {
	r.Created = time.Unix(r.CreatedUnix, 0).Local()
	return nil
}

// APIFormat returns the API format of the release. It assumes some fields have
// been assigned with values:
// Required - Publisher
func (r *Release) APIFormat() *apiv1types.RepositoryRelease {
	return &apiv1types.RepositoryRelease{
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

func createTag(ctx context.Context, gitRepo *git.Repository, r *Release) error {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	// Only actually create the tag when publishing.
	if !r.IsDraft {
		if !gitRepo.HasTag(ctx, r.TagName) {
			commit, err := gitRepo.BranchCommit(ctx, r.Target)
			if err != nil {
				return errors.Newf("get branch commit: %v", err)
			}

			// 🚨 SECURITY: Trim any leading '-' to prevent command line argument injection.
			r.TagName = strings.TrimLeft(r.TagName, "-")
			if err = gitRepo.CreateTag(ctx, r.TagName, commit.ID.String()); err != nil {
				if strings.Contains(err.Error(), "is not a valid tag name") {
					return ErrInvalidTagName{r.TagName}
				}
				return err
			}
		} else {
			commit, err := gitRepo.TagCommit(ctx, r.TagName)
			if err != nil {
				return errors.Newf("get tag commit: %v", err)
			}

			r.Sha1 = commit.ID.String()
			r.NumCommits, err = commit.CommitsCount(ctx)
			if err != nil {
				return errors.Newf("count commits: %v", err)
			}
		}
	}
	return nil
}

var _ errx.NotFound = (*ErrReleaseNotExist)(nil)

type ErrReleaseNotExist struct {
	args map[string]any
}

func IsErrReleaseNotExist(err error) bool {
	_, ok := err.(ErrReleaseNotExist)
	return ok
}

func (err ErrReleaseNotExist) Error() string {
	return fmt.Sprintf("release does not exist: %v", err.args)
}

func (ErrReleaseNotExist) NotFound() bool {
	return true
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
