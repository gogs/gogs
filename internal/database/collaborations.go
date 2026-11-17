package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
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
// repository, and recalculates the accesses of the repository. Adding a
// collaborator that the repository has is a no-op.
func (s *CollaborationsStore) Add(ctx context.Context, repo *Repository, userID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c := &Collaboration{RepoID: repo.ID, UserID: userID}
		result := tx.Where("repo_id = ? AND user_id = ?", repo.ID, userID).Attrs(Collaboration{Mode: AccessModeWrite}).FirstOrCreate(c)
		if result.Error != nil {
			return errors.Wrap(result.Error, "upsert")
		} else if result.RowsAffected <= 0 {
			return nil
		}
		return newPermissionsStore(tx).recalculateAccesses(ctx, repo)
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

// ChangeMode sets the access mode of the collaborator with given user ID to
// the repository. The access mode of the actor bounds the new mode, so an
// actor can never grant a level higher than their own, and collaborators hold
// at most admin access. Changing the mode of a user who is not a collaborator
// is a no-op. For a repository of an organization, the access of the
// collaborator is the highest of the new mode and the modes of their teams.
func (s *CollaborationsStore) ChangeMode(ctx context.Context, repo *Repository, actorMode AccessMode, userID int64, mode AccessMode) error {
	if mode <= AccessModeNone || mode > AccessModeAdmin || mode > actorMode {
		return nil
	}

	db := s.db.WithContext(ctx)
	collaboration := new(Collaboration)
	err := db.Where("repo_id = ? AND user_id = ?", repo.ID, userID).First(collaboration).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return errors.Wrap(err, "get collaboration")
	} else if collaboration.Mode == mode {
		return nil
	}
	collaboration.Mode = mode

	if err = repo.getOwner(db); err != nil {
		return errors.Wrap(err, "get owner")
	}
	if repo.Owner.IsOrganization() {
		teams, err := GetUserTeams(repo.OwnerID, userID)
		if err != nil {
			return errors.Wrapf(err, "list teams of user %d in organization %d", userID, repo.OwnerID)
		}
		for _, t := range teams {
			mode = max(mode, t.Authorize)
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(collaboration).Update("mode", collaboration.Mode).Error
		if err != nil {
			return errors.Wrap(err, "update collaboration")
		}

		access := &Access{UserID: userID, RepoID: repo.ID}
		err = tx.Where("user_id = ? AND repo_id = ?", userID, repo.ID).Assign(Access{Mode: mode}).FirstOrCreate(access).Error
		if err != nil {
			return errors.Wrap(err, "upsert access")
		}
		return nil
	})
}

// Delete removes the user with given ID from the collaborators of the
// repository, and recalculates the accesses of the repository. Deleting a user
// who is not a collaborator is a no-op.
func (s *CollaborationsStore) Delete(ctx context.Context, repo *Repository, userID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("repo_id = ? AND user_id = ?", repo.ID, userID).Delete(new(Collaboration))
		if result.Error != nil {
			return errors.Wrap(result.Error, "delete")
		} else if result.RowsAffected <= 0 {
			return nil
		}
		return newPermissionsStore(tx).recalculateAccesses(ctx, repo)
	})
}
