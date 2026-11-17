package database

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
)

// MirrorsStore is the storage layer for the sync settings of mirror
// repositories.
type MirrorsStore struct {
	db *gorm.DB
}

func newMirrorsStore(db *gorm.DB) *MirrorsStore {
	return &MirrorsStore{db: db}
}

// GetByRepoID returns the mirror of the repository with given ID, with the
// repository and its attributes. It returns MirrorNotExist when not found.
func (s *MirrorsStore) GetByRepoID(ctx context.Context, repoID int64) (*Mirror, error) {
	m := new(Mirror)
	err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).First(m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, MirrorNotExist{RepoID: repoID}
		}
		return nil, err
	}

	m.Repo, err = newReposStore(s.db).getWithAttributes(ctx, repoID)
	if err != nil {
		return nil, errors.Wrap(err, "get repository")
	}
	return m, nil
}

// LoadForRepos sets the mirrors of given repositories that are mirrors.
// Repositories without a mirror are left with none.
func (s *MirrorsStore) LoadForRepos(ctx context.Context, repos ...*Repository) error {
	repoIDs := make([]int64, 0, len(repos))
	for _, repo := range repos {
		if repo.IsMirror {
			repoIDs = append(repoIDs, repo.ID)
		}
	}
	if len(repoIDs) == 0 {
		return nil
	}

	var mirrors []*Mirror
	err := s.db.WithContext(ctx).Where("repo_id IN ?", repoIDs).Find(&mirrors).Error
	if err != nil {
		return errors.Wrap(err, "list mirrors")
	}
	byRepoID := make(map[int64]*Mirror, len(mirrors))
	for _, m := range mirrors {
		byRepoID[m.RepoID] = m
	}
	for _, repo := range repos {
		if !repo.IsMirror {
			continue
		}
		m := byRepoID[repo.ID]
		if m != nil {
			m.Repo = repo
		}
		repo.Mirror = m
	}
	return nil
}

// Update saves all fields of the mirror.
func (s *MirrorsStore) Update(ctx context.Context, m *Mirror) error {
	return s.db.WithContext(ctx).Model(m).Select("*").Updates(m).Error
}

// DeleteByRepoID deletes the mirror of the repository with given ID, which
// makes it a regular repository. Deleting a mirror that does not exist is not
// an error.
func (s *MirrorsStore) DeleteByRepoID(ctx context.Context, repoID int64) error {
	return s.db.WithContext(ctx).Where("repo_id = ?", repoID).Delete(new(Mirror)).Error
}

// listDueRepoIDs returns the IDs of the repositories whose mirrors are due to
// sync at given time. Mirrors whose repository is gone are logged and skipped.
func (s *MirrorsStore) listDueRepoIDs(ctx context.Context, logger *logx.Logger, now time.Time) ([]int64, error) {
	var mirrors []*Mirror
	err := s.db.WithContext(ctx).
		Where("next_update_unix <= ?", now.Unix()).
		Order("next_update_unix ASC").
		Find(&mirrors).
		Error
	if err != nil {
		return nil, err
	}
	if len(mirrors) == 0 {
		return nil, nil
	}

	repoIDs := make([]int64, 0, len(mirrors))
	for _, m := range mirrors {
		repoIDs = append(repoIDs, m.RepoID)
	}
	var existing []int64
	err = s.db.WithContext(ctx).Model(new(Repository)).Where("id IN ?", repoIDs).Pluck("id", &existing).Error
	if err != nil {
		return nil, errors.Wrap(err, "list repositories")
	}
	exists := make(map[int64]bool, len(existing))
	for _, id := range existing {
		exists[id] = true
	}

	dueRepoIDs := make([]int64, 0, len(mirrors))
	for _, m := range mirrors {
		if !exists[m.RepoID] {
			logger.Error("Disconnected mirror repository found", "mirrorID", m.ID)
			continue
		}
		dueRepoIDs = append(dueRepoIDs, m.RepoID)
	}
	return dueRepoIDs, nil
}

// markSynced records that the mirror of the repository with given ID synced at
// given time.
func (s *MirrorsStore) markSynced(ctx context.Context, repoID int64, at time.Time) error {
	return s.db.WithContext(ctx).
		Model(new(Mirror)).
		Where("repo_id = ?", repoID).
		UpdateColumn("updated_unix", at.Unix()).
		Error
}
