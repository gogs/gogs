package database

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
)

// Release represents a release of repository.
type Release struct {
	ID               int64
	RepoID           int64
	Repo             *Repository `json:"-" gorm:"-"`
	PublisherID      int64
	Publisher        *User `json:"-" gorm:"-"`
	TagName          string
	LowerTagName     string
	Target           string
	Title            string
	Sha1             string `gorm:"type:VARCHAR(64)"`
	NumCommits       int64
	NumCommitsBehind int64  `json:"-" gorm:"-"`
	Note             string `gorm:"type:TEXT"`
	IsDraft          bool   `gorm:"not null;default:FALSE"`
	IsPrerelease     bool

	Created     time.Time `json:"-" gorm:"-"`
	CreatedUnix int64

	Attachments []*Attachment `json:"-" gorm:"-"`
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
func (r *Release) APIFormat() *v1types.RepositoryRelease {
	return &v1types.RepositoryRelease{
		ID:              r.ID,
		TagName:         r.TagName,
		TargetCommitish: r.Target,
		Name:            r.Title,
		Body:            r.Note,
		Draft:           r.IsDraft,
		Prerelease:      r.IsPrerelease,
		Author:          r.Publisher.APIFormat(),
		Created:         r.Created,
		Assets:          releaseAssets(r.Attachments),
	}
}

// releaseAssets returns the API format of the attachments of a release.
func releaseAssets(attachments []*Attachment) []*v1types.ReleaseAsset {
	assets := make([]*v1types.ReleaseAsset, len(attachments))
	for i, a := range attachments {
		assets[i] = a.APIFormat()
	}
	return assets
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

// ReleasesStore is the storage layer for releases of repositories.
type ReleasesStore struct {
	db *gorm.DB
}

func newReleasesStore(db *gorm.DB) *ReleasesStore {
	return &ReleasesStore{db: db}
}

// exists returns true if the repository with given ID has a release of given
// tag name, regardless of its case.
func (s *ReleasesStore) exists(ctx context.Context, repoID int64, tagName string) (bool, error) {
	if tagName == "" {
		return false, nil
	}

	var count int64
	err := s.db.WithContext(ctx).Model(new(Release)).
		Where("repo_id = ? AND lower_tag_name = ?", repoID, strings.ToLower(tagName)).
		Count(&count).Error
	return count > 0, err
}

// Create creates the release, its tag when it is not a draft and the tag does
// not exist yet, and attaches the unattached attachments with given UUIDs to
// it. It returns ErrReleaseAlreadyExist when the repository has a release of
// the same tag name, regardless of its case.
func (s *ReleasesStore) Create(ctx context.Context, logger *logx.Logger, gitRepo *git.Repository, r *Release, uuids []string) error {
	// Stopping midway, e.g., when the client disconnects, would leave the tag
	// without its release.
	ctx = context.WithoutCancel(ctx)

	exists, err := s.exists(ctx, r.RepoID, r.TagName)
	if err != nil {
		return errors.Wrap(err, "check existence")
	} else if exists {
		return ErrReleaseAlreadyExist{r.TagName}
	}

	if err = createTag(ctx, gitRepo, r); err != nil {
		return err
	}
	r.LowerTagName = strings.ToLower(r.TagName)

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Create(r).Error
		if err != nil {
			return errors.Wrap(err, "create release")
		}
		return newAttachmentsStore(tx).bindToRelease(ctx, r.ID, uuids)
	})
	if err != nil {
		return err
	}

	if r.IsDraft {
		return nil
	}
	created, err := s.GetByID(ctx, r.ID)
	if err != nil {
		return errors.Wrap(err, "get created release")
	}
	s.preparePublishWebhooks(ctx, logger, created)
	return nil
}

// preparePublishWebhooks prepares the deliveries of the event that the release
// is published. Failing to prepare them does not fail publishing.
func (s *ReleasesStore) preparePublishWebhooks(ctx context.Context, logger *logx.Logger, r *Release) {
	err := s.LoadAttributes(ctx, r)
	if err == nil {
		err = newWebhooksStore(s.db).Prepare(ctx, logger, r.Repo, HookEventTypeRelease, &v1types.WebhookReleasePayload{
			Action:     v1types.WebhookReleasePublished,
			Release:    r.APIFormat(),
			Repository: r.Repo.APIFormatLegacy(logger, nil),
			Sender:     r.Publisher.APIFormat(),
		})
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "releaseID", r.ID, "error", err)
	}
}

func (s *ReleasesStore) get(ctx context.Context, args map[string]any, query string, values ...any) (*Release, error) {
	r := new(Release)
	err := s.db.WithContext(ctx).Where(query, values...).First(r).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReleaseNotExist{args: args}
		}
		return nil, err
	}
	return r, nil
}

// GetByTagName returns the release of given tag name, regardless of its case,
// of the repository with given ID. It returns ErrReleaseNotExist when not
// found.
func (s *ReleasesStore) GetByTagName(ctx context.Context, repoID int64, tagName string) (*Release, error) {
	return s.get(ctx, map[string]any{"repoID": repoID, "tag": tagName}, "repo_id = ? AND lower_tag_name = ?", repoID, strings.ToLower(tagName))
}

// GetByID returns the release with given ID. It returns ErrReleaseNotExist
// when not found.
func (s *ReleasesStore) GetByID(ctx context.Context, id int64) (*Release, error) {
	return s.get(ctx, map[string]any{"releaseID": id}, "id = ?", id)
}

// ListPublishedByRepoID returns the published releases of the repository with
// given ID, newest first. When tagNames is not empty, only releases of those
// tag names are returned.
func (s *ReleasesStore) ListPublishedByRepoID(ctx context.Context, repoID int64, tagNames ...string) ([]*Release, error) {
	db := s.db.WithContext(ctx).Where("repo_id = ? AND is_draft = ?", repoID, false)
	if len(tagNames) > 0 {
		db = db.Where("tag_name IN ?", tagNames)
	}
	releases := make([]*Release, 0, 5)
	return releases, db.Order("created_unix DESC").Order("id DESC").Find(&releases).Error
}

// ListByRepoID returns all releases, including drafts, of the repository with
// given ID.
func (s *ReleasesStore) ListByRepoID(ctx context.Context, repoID int64) ([]*Release, error) {
	releases := make([]*Release, 0)
	return releases, s.db.WithContext(ctx).Where("repo_id = ?", repoID).Order("id").Find(&releases).Error
}

// ListDraftsByRepoID returns the draft releases of the repository with given
// ID.
func (s *ReleasesStore) ListDraftsByRepoID(ctx context.Context, repoID int64) ([]*Release, error) {
	releases := make([]*Release, 0)
	return releases, s.db.WithContext(ctx).Where("repo_id = ? AND is_draft = ?", repoID, true).Order("id").Find(&releases).Error
}

// LoadAttributes sets the repository with its attributes, the publisher and
// the attachments of given releases that are not set yet. Releases whose
// publisher no longer exists get the ghost user.
func (s *ReleasesStore) LoadAttributes(ctx context.Context, releases ...*Release) error {
	repos := newReposStore(s.db)
	for _, r := range releases {
		if r.Repo == nil {
			repo, err := repos.GetByID(ctx, r.RepoID)
			if err != nil {
				return errors.Wrapf(err, "get repository %d", r.RepoID)
			}
			if err = repos.LoadAttributes(ctx, repo); err != nil {
				return errors.Wrapf(err, "load repository %d", r.RepoID)
			}
			r.Repo = repo
		}

		if r.Publisher == nil {
			publisher, err := newUsersStore(s.db).GetByID(ctx, r.PublisherID)
			if err != nil {
				if !IsErrUserNotExist(err) {
					return errors.Wrapf(err, "get publisher %d", r.PublisherID)
				}
				r.PublisherID = -1
				publisher = NewGhostUser()
			}
			r.Publisher = publisher
		}

		if r.Attachments == nil {
			attachments, err := newAttachmentsStore(s.db).ListByReleaseID(ctx, r.ID)
			if err != nil {
				return errors.Wrapf(err, "list attachments of release %d", r.ID)
			}
			r.Attachments = attachments
		}
	}
	return nil
}

// Update saves all fields of the release, which the doer publishes when
// isPublish is true, and creates its tag when it is published and the tag does
// not exist yet. The release keeps only the attachments with given UUIDs that
// it had or that are not attached to anything else, and the others are
// detached from it.
func (s *ReleasesStore) Update(ctx context.Context, logger *logx.Logger, doer *User, gitRepo *git.Repository, r *Release, isPublish bool, uuids []string) error {
	// Stopping midway, e.g., when the client disconnects, would leave the tag
	// without its release being published.
	ctx = context.WithoutCancel(ctx)

	// Not wrapped, so that callers recognize ErrInvalidTagName.
	err := createTag(ctx, gitRepo, r)
	if err != nil {
		return err
	}

	r.PublisherID = doer.ID
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(r).Select("*").Updates(r).Error
		if err != nil {
			return errors.Wrap(err, "update release")
		}
		// The release may have been deleted since it was read. Files must not be
		// attached to it then. The update above waits for a deletion in progress.
		var count int64
		if err = tx.Model(new(Release)).Where("id = ?", r.ID).Count(&count).Error; err != nil {
			return errors.Wrap(err, "count release")
		} else if count == 0 {
			return ErrReleaseNotExist{args: map[string]any{"releaseID": r.ID}}
		}

		// Detach all attachments and attach back those that stay.
		err = tx.Model(new(Attachment)).Where("release_id = ?", r.ID).Update("release_id", 0).Error
		if err != nil {
			return errors.Wrap(err, "detach attachments")
		}
		return newAttachmentsStore(tx).bindToRelease(ctx, r.ID, uuids)
	})
	if err != nil {
		return err
	}

	if !isPublish {
		return nil
	}
	r.Publisher = doer
	// The attachments changed, so they are listed again.
	r.Attachments = nil
	s.preparePublishWebhooks(ctx, logger, r)
	return nil
}

// Attach attaches the unattached attachment with given ID to the release with
// given ID. It returns ErrReleaseNotExist when the release does not exist,
// e.g., it was deleted since it was read.
func (s *ReleasesStore) Attach(ctx context.Context, releaseID, attachmentID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The update waits for a deletion of the release in progress, and a
		// deletion that starts later waits for this to finish and deletes the
		// attachment with the release.
		err := tx.Model(new(Release)).Where("id = ?", releaseID).Update("repo_id", gorm.Expr("repo_id")).Error
		if err != nil {
			return errors.Wrap(err, "lock release")
		}
		var count int64
		if err = tx.Model(new(Release)).Where("id = ?", releaseID).Count(&count).Error; err != nil {
			return errors.Wrap(err, "count release")
		} else if count == 0 {
			return ErrReleaseNotExist{args: map[string]any{"releaseID": releaseID}}
		}
		return tx.Model(new(Attachment)).Where(unboundAttachmentCond).Where("id = ?", attachmentID).Update("release_id", releaseID).Error
	})
}

// DeleteByID deletes the release with given ID, its tag and its attachments
// when it belongs to the repository with given ID. It returns
// ErrReleaseNotExist when the release does not exist.
func (s *ReleasesStore) DeleteByID(ctx context.Context, logger *logx.Logger, repoID, id int64) error {
	// Stopping midway, e.g., when the client disconnects, would leave the
	// release without its tag.
	ctx = context.WithoutCancel(ctx)

	r, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if r.RepoID != repoID {
		return nil
	}
	if err = s.LoadAttributes(ctx, r); err != nil {
		return errors.Wrap(err, "load attributes")
	}

	err = git.RepoDeleteTag(ctx, r.Repo.RepoPath(), r.TagName)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return errors.Wrap(err, "delete tag")
	}
	// The release row is deleted first, which waits for an edit of the release
	// that is in progress. The attachments are listed after that, so that the
	// rows deleted and the files removed include those the edit attached.
	var attachments []*Attachment
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Delete(new(Release), r.ID).Error
		if err != nil {
			return errors.Wrap(err, "delete release")
		}
		if err = tx.Where("release_id = ?", r.ID).Find(&attachments).Error; err != nil {
			return errors.Wrap(err, "list attachments")
		}
		for _, a := range attachments {
			if err = tx.Delete(new(Attachment), a.ID).Error; err != nil {
				return errors.Wrapf(err, "delete attachment %d", a.ID)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Files are removed only once their rows are gone, so a failure cannot leave
	// rows whose files are missing. The release is gone by then, so failing to
	// remove a file does not fail the deletion.
	for _, a := range attachments {
		if err = os.Remove(a.LocalPath()); err != nil && !os.IsNotExist(err) {
			logger.Error("Failed to remove file of attachment", "attachmentID", a.ID, "error", err)
		}
	}
	return nil
}
