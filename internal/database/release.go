package database

import (
	"fmt"
	"sort"
	"strings"
	"time"

	log "unknwon.dev/clog/v2"

	"github.com/cockroachdb/errors"
	"github.com/gogs/git-module"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
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
	Sha1             string `xorm:"VARCHAR(40)"`
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

func (r *Release) loadAttributes(tx *gorm.DB) (err error) {
	if r.Repo == nil {
		r.Repo, err = getRepositoryByID(tx, r.RepoID)
		if err != nil {
			return errors.Newf("getRepositoryByID [repo_id: %d]: %v", r.RepoID, err)
		}
	}

	if r.Publisher == nil {
		r.Publisher, err = getUserByID(tx, r.PublisherID)
		if err != nil {
			if IsErrUserNotExist(err) {
				r.PublisherID = -1
				r.Publisher = NewGhostUser()
			} else {
				return errors.Newf("getUserByID.(Publisher) [publisher_id: %d]: %v", r.PublisherID, err)
			}
		}
	}

	if r.Attachments == nil {
		r.Attachments, err = getAttachmentsByReleaseID(tx, r.ID)
		if err != nil {
			return errors.Newf("getAttachmentsByReleaseID [%d]: %v", r.ID, err)
		}
	}

	return nil
}

func (r *Release) LoadAttributes() error {
	return r.loadAttributes(Handle.db)
}

// This method assumes some fields assigned with values:
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

// IsReleaseExist returns true if release with given tag name already exists.
func IsReleaseExist(repoID int64, tagName string) (bool, error) {
	if tagName == "" {
		return false, nil
	}

	var count int64
	err := Handle.db.Model(new(Release)).Where(&Release{RepoID: repoID, LowerTagName: strings.ToLower(tagName)}).Count(&count).Error
	return count > 0, err
}

func createTag(gitRepo *git.Repository, r *Release) error {
	// Only actual create when publish.
	if !r.IsDraft {
		if !gitRepo.HasTag(r.TagName) {
			commit, err := gitRepo.BranchCommit(r.Target)
			if err != nil {
				return errors.Newf("get branch commit: %v", err)
			}

			// 🚨 SECURITY: Trim any leading '-' to prevent command line argument injection.
			r.TagName = strings.TrimLeft(r.TagName, "-")
			if err = gitRepo.CreateTag(r.TagName, commit.ID.String()); err != nil {
				if strings.Contains(err.Error(), "is not a valid tag name") {
					return ErrInvalidTagName{r.TagName}
				}
				return err
			}
		} else {
			commit, err := gitRepo.TagCommit(r.TagName)
			if err != nil {
				return errors.Newf("get tag commit: %v", err)
			}

			r.Sha1 = commit.ID.String()
			r.NumCommits, err = commit.CommitsCount()
			if err != nil {
				return errors.Newf("count commits: %v", err)
			}
		}
	}
	return nil
}

func (r *Release) preparePublishWebhooks() {
	if err := PrepareWebhooks(r.Repo, HookEventTypeRelease, &apiv1types.WebhookReleasePayload{
		Action:     apiv1types.WebhookReleasePublished,
		Release:    r.APIFormat(),
		Repository: r.Repo.APIFormatLegacy(nil),
		Sender:     r.Publisher.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks: %v", err)
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

	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(r).Error; err != nil {
			return errors.Newf("insert: %v", err)
		}

		if len(uuids) > 0 {
			if err := tx.Model(new(Attachment)).Where(unboundAttachmentCond).Where("uuid IN ?", uuids).Update("release_id", r.ID).Error; err != nil {
				return errors.Newf("link attachments: %v", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Only send webhook when actually published, skip drafts
	if r.IsDraft {
		return nil
	}
	r, err = GetReleaseByID(r.ID)
	if err != nil {
		return errors.Newf("GetReleaseByID: %v", err)
	}
	r.preparePublishWebhooks()
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

// GetRelease returns release by given ID.
func GetRelease(repoID int64, tagName string) (*Release, error) {
	isExist, err := IsReleaseExist(repoID, tagName)
	if err != nil {
		return nil, err
	} else if !isExist {
		return nil, ErrReleaseNotExist{args: map[string]any{"tag": tagName}}
	}

	r := new(Release)
	if err = Handle.db.Where(&Release{RepoID: repoID, LowerTagName: strings.ToLower(tagName)}).First(r).Error; err != nil {
		return nil, errors.Newf("get: %v", err)
	}

	return r, r.LoadAttributes()
}

// GetReleaseByID returns release with given ID.
func GetReleaseByID(id int64) (*Release, error) {
	r := new(Release)
	err := Handle.db.Where("id = ?", id).First(r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReleaseNotExist{args: map[string]any{"releaseID": id}}
		}
		return nil, err
	}

	return r, r.LoadAttributes()
}

// GetPublishedReleasesByRepoID returns a list of published releases of repository.
// If matches is not empty, only published releases in matches will be returned.
// In any case, drafts won't be returned by this function.
func GetPublishedReleasesByRepoID(repoID int64, matches ...string) ([]*Release, error) {
	query := Handle.db.Where("repo_id = ? AND is_draft = ?", repoID, false).Order("created_unix DESC")
	if len(matches) > 0 {
		query = query.Where("tag_name IN ?", matches)
	}
	releases := make([]*Release, 0, 5)
	return releases, query.Find(&releases).Error
}

// GetReleasesByRepoID returns a list of all releases (including drafts) of given repository.
func GetReleasesByRepoID(repoID int64) ([]*Release, error) {
	releases := make([]*Release, 0)
	return releases, Handle.db.Where("repo_id = ?", repoID).Find(&releases).Error
}

// GetDraftReleasesByRepoID returns all draft releases of repository.
func GetDraftReleasesByRepoID(repoID int64) ([]*Release, error) {
	releases := make([]*Release, 0)
	return releases, Handle.db.Where("repo_id = ? AND is_draft = ?", repoID, true).Find(&releases).Error
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
		return errors.Newf("createTag: %v", err)
	}

	r.PublisherID = doer.ID

	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(r).Select("*").Updates(r).Error; err != nil {
			return errors.Newf("Update: %v", err)
		}

		// Unlink all current attachments and link back later if still valid
		if err := tx.Exec("UPDATE attachment SET release_id = 0 WHERE release_id = ?", r.ID).Error; err != nil {
			return errors.Newf("unlink current attachments: %v", err)
		}

		if len(uuids) > 0 {
			if err := tx.Model(new(Attachment)).Where(unboundAttachmentCond).Where("uuid IN ?", uuids).Update("release_id", r.ID).Error; err != nil {
				return errors.Newf("link attachments: %v", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
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
		return errors.Newf("GetReleaseByID: %v", err)
	}

	// Mark sure the delete operation against same repository.
	if repoID != rel.RepoID {
		return nil
	}

	repo, err := GetRepositoryByID(rel.RepoID)
	if err != nil {
		return errors.Newf("GetRepositoryByID: %v", err)
	}

	gitRepo, err := git.Open(repo.RepoPath())
	if err != nil {
		return errors.Newf("open repository: %v", err)
	}
	err = gitRepo.DeleteTag(rel.TagName)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return errors.Newf("delete tag: %v", err)
	}

	if err = Handle.db.Where("id = ?", rel.ID).Delete(new(Release)).Error; err != nil {
		return errors.Newf("delete: %v", err)
	}

	return nil
}
