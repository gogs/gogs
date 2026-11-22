package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/errx"
)

// CollaborationsStore is the storage layer for collaborators of repositories.
type CollaborationsStore struct {
	db *gorm.DB
}

func newCollaborationsStore(db *gorm.DB) *CollaborationsStore {
	return &CollaborationsStore{db: db}
}

// IsCollaborator returns true if the user with given ID is a collaborator of
// the repository with given ID. Failing to check is logged and returns false.
func (s *CollaborationsStore) IsCollaborator(ctx context.Context, logger *logx.Logger, repoID, userID int64) bool {
	var count int64
	err := s.db.WithContext(ctx).Model(new(Collaboration)).Where("repo_id = ? AND user_id = ?", repoID, userID).Count(&count).Error
	if err != nil {
		logger.Error("Failed to get collaboration", "repoID", repoID, "userID", userID, "error", err)
		return false
	}
	return count > 0
}

// Add adds the user with given ID as a collaborator with write access to the
// repository, and recalculates their access to it. Adding a collaborator that
// the repository has keeps their mode.
func (s *CollaborationsStore) Add(ctx context.Context, repo *Repository, userID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Deleting the repository meanwhile would leave the collaboration and
		// access behind.
		if _, exists, err := lockRepository(tx, repo.ID); err != nil {
			return err
		} else if !exists {
			return ErrRepoNotExist{args: errx.Args{"repoID": repo.ID}}
		}

		// The number of affected rows does not tell whether the row is new,
		// because MariaDB counts the existing row returned.
		c := &Collaboration{RepoID: repo.ID, UserID: userID, Mode: AccessModeWrite}
		err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(c).Error
		if err != nil {
			return errors.Wrap(err, "insert")
		}
		return newPermissionsStore(tx).recalculateUserAccess(ctx, repo, userID)
	})
}

// listByRepoID returns the collaborations of the repository with given ID.
func (s *CollaborationsStore) listByRepoID(ctx context.Context, repoID int64) ([]*Collaboration, error) {
	collaborations := make([]*Collaboration, 0, 5)
	return collaborations, s.db.WithContext(ctx).Where("repo_id = ?", repoID).Order("id ASC").Find(&collaborations).Error
}

// List returns the collaborators of the repository with given ID, in the order
// they were added.
func (s *CollaborationsStore) List(ctx context.Context, repoID int64) ([]*Collaborator, error) {
	collaborations, err := s.listByRepoID(ctx, repoID)
	if err != nil {
		return nil, errors.Wrap(err, "list collaborations")
	}

	collaborators := make([]*Collaborator, len(collaborations))
	for i, c := range collaborations {
		user, err := newUsersStore(s.db).GetByID(ctx, c.UserID)
		if err != nil {
			return nil, errors.Wrapf(err, "get user %d", c.UserID)
		}
		collaborators[i] = &Collaborator{
			User:          user,
			Collaboration: c,
		}
	}
	return collaborators, nil
}

// ChangeMode sets the collaborator's access mode to the repository, capped at
// admin and at the actor's own mode. It is a no-op for a user who is not a
// collaborator. In a repository of an organization, the collaborator gets the
// highest of this mode and the modes of their teams that have the repository.
func (s *CollaborationsStore) ChangeMode(ctx context.Context, repo *Repository, actorMode AccessMode, userID int64, mode AccessMode) error {
	if mode <= AccessModeNone || mode > AccessModeAdmin || mode > actorMode {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(new(Collaboration)).Where("repo_id = ? AND user_id = ?", repo.ID, userID).Update("mode", mode)
		if result.Error != nil {
			return errors.Wrap(result.Error, "update")
		} else if result.RowsAffected <= 0 {
			return nil
		}
		return newPermissionsStore(tx).recalculateUserAccess(ctx, repo, userID)
	})
}

// Delete removes the user with given ID from the collaborators of the
// repository, and recalculates their access to it. Deleting a user who is not
// a collaborator is a no-op.
func (s *CollaborationsStore) Delete(ctx context.Context, repo *Repository, userID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("repo_id = ? AND user_id = ?", repo.ID, userID).Delete(new(Collaboration))
		if result.Error != nil {
			return errors.Wrap(result.Error, "delete")
		} else if result.RowsAffected <= 0 {
			return nil
		}
		return newPermissionsStore(tx).recalculateUserAccess(ctx, repo, userID)
	})
}
