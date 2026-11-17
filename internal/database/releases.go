package database

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/git"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

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
		err = newWebhooksStore(s.db).Prepare(ctx, logger, r.Repo, HookEventTypeRelease, &apiv1types.WebhookReleasePayload{
			Action:     apiv1types.WebhookReleasePublished,
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

	err := createTag(ctx, gitRepo, r)
	if err != nil {
		return errors.Wrap(err, "create tag")
	}

	r.PublisherID = doer.ID
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(r).Select("*").Updates(r).Error
		if err != nil {
			return errors.Wrap(err, "update release")
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
	s.preparePublishWebhooks(ctx, logger, r)
	return nil
}

// DeleteByID deletes the release with given ID and its tag when it belongs to
// the repository with given ID. Its attachments are kept. It returns
// ErrReleaseNotExist when the release does not exist.
func (s *ReleasesStore) DeleteByID(ctx context.Context, repoID, id int64) error {
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
	return s.db.WithContext(ctx).Delete(new(Release), r.ID).Error
}
