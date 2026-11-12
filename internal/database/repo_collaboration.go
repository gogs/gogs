package database

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// Collaboration represent the relation between an individual and a repository.
type Collaboration struct {
	ID     int64      `gorm:"primary_key"`
	UserID int64      `gorm:"not null;uniqueIndex:UQE_collaboration_s;index:IDX_collaboration_user_id"`
	RepoID int64      `gorm:"not null;uniqueIndex:UQE_collaboration_s;index:IDX_collaboration_repo_id"`
	Mode   AccessMode `gorm:"size:32;not null;default:2"`
}

func (c *Collaboration) ModeI18nKey() string {
	switch c.Mode {
	case AccessModeRead:
		return "repo.settings.collaboration.read"
	case AccessModeWrite:
		return "repo.settings.collaboration.write"
	case AccessModeAdmin:
		return "repo.settings.collaboration.admin"
	default:
		return "repo.settings.collaboration.undefined"
	}
}

// IsCollaborator returns true if the user is a collaborator of the repository.
func IsCollaborator(repoID, userID int64) bool {
	var count int64
	err := Handle.db.Model(new(Collaboration)).Where(&Collaboration{RepoID: repoID, UserID: userID}).Count(&count).Error
	if err != nil {
		log.Error("get collaboration [repo_id: %d, user_id: %d]: %v", repoID, userID, err)
		return false
	}
	return count > 0
}

func (r *Repository) IsCollaborator(userID int64) bool {
	return IsCollaborator(r.ID, userID)
}

// AddCollaborator adds new collaboration to a repository with default access mode.
func (r *Repository) AddCollaborator(u *User) error {
	if IsCollaborator(r.ID, u.ID) {
		return nil
	}

	return Handle.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Create(&Collaboration{RepoID: r.ID, UserID: u.ID, Mode: AccessModeWrite}).Error
		if err != nil {
			return err
		} else if err = r.recalculateAccesses(tx); err != nil {
			return errors.Newf("recalculateAccesses [repo_id: %v]: %v", r.ID, err)
		}
		return nil
	})
}

func (r *Repository) getCollaborations(tx *gorm.DB) ([]*Collaboration, error) {
	collaborations := make([]*Collaboration, 0)
	return collaborations, tx.Where("repo_id = ?", r.ID).Find(&collaborations).Error
}

// Collaborator represents a user with collaboration details.
type Collaborator struct {
	*User
	Collaboration *Collaboration
}

func (c *Collaborator) APIFormat() *apiv1types.RepositoryCollaborator {
	return &apiv1types.RepositoryCollaborator{
		User: c.User.APIFormat(),
		Permissions: apiv1types.RepositoryPermission{
			Admin: c.Collaboration.Mode >= AccessModeAdmin,
			Push:  c.Collaboration.Mode >= AccessModeWrite,
			Pull:  c.Collaboration.Mode >= AccessModeRead,
		},
	}
}

func (r *Repository) getCollaborators(tx *gorm.DB) ([]*Collaborator, error) {
	collaborations, err := r.getCollaborations(tx)
	if err != nil {
		return nil, errors.Newf("getCollaborations: %v", err)
	}

	collaborators := make([]*Collaborator, len(collaborations))
	for i, c := range collaborations {
		user, err := getUserByID(tx, c.UserID)
		if err != nil {
			return nil, err
		}
		collaborators[i] = &Collaborator{
			User:          user,
			Collaboration: c,
		}
	}
	return collaborators, nil
}

// GetCollaborators returns the collaborators for a repository
func (r *Repository) GetCollaborators() ([]*Collaborator, error) {
	return r.getCollaborators(Handle.db)
}

// ChangeCollaborationAccessMode sets new access mode for the collaboration.
// The actor's access mode bounds the new mode, so an actor can never grant a
// level higher than their own.
func (r *Repository) ChangeCollaborationAccessMode(actorMode AccessMode, userID int64, mode AccessMode) error {
	// Collaborators can hold at most admin access.
	if mode <= AccessModeNone || mode > AccessModeAdmin {
		return nil
	}

	// Actors must not grant a level above their own.
	if mode > actorMode {
		return nil
	}

	collaboration := new(Collaboration)
	err := Handle.db.Where(&Collaboration{RepoID: r.ID, UserID: userID}).First(collaboration).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return errors.Newf("get collaboration: %v", err)
	}

	if collaboration.Mode == mode {
		return nil
	}
	collaboration.Mode = mode

	// If it's an organizational repository, merge with team access level for highest permission
	if r.Owner.IsOrganization() {
		teams, err := GetUserTeams(r.OwnerID, userID)
		if err != nil {
			return errors.Newf("GetUserTeams: [org_id: %d, user_id: %d]: %v", r.OwnerID, userID, err)
		}
		for i := range teams {
			if mode < teams[i].Authorize {
				mode = teams[i].Authorize
			}
		}
	}

	return Handle.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(collaboration).Select("*").Updates(collaboration).Error; err != nil {
			return errors.Newf("update collaboration: %v", err)
		}

		var count int64
		err := tx.Model(new(Access)).Where("user_id = ? AND repo_id = ?", userID, r.ID).Count(&count).Error
		if err != nil {
			return errors.Newf("get access record: %v", err)
		}
		if count > 0 {
			err = tx.Exec("UPDATE access SET mode = ? WHERE user_id = ? AND repo_id = ?", mode, userID, r.ID).Error
		} else {
			err = tx.Create(&Access{UserID: userID, RepoID: r.ID, Mode: mode}).Error
		}
		if err != nil {
			return errors.Newf("update/insert access table: %v", err)
		}
		return nil
	})
}

// DeleteCollaboration removes collaboration relation between the user and repository.
func DeleteCollaboration(repo *Repository, userID int64) (err error) {
	if !IsCollaborator(repo.ID, userID) {
		return nil
	}

	return Handle.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where(&Collaboration{RepoID: repo.ID, UserID: userID}).Delete(new(Collaboration))
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return repo.recalculateAccesses(tx)
	})
}

func (r *Repository) DeleteCollaboration(userID int64) error {
	return DeleteCollaboration(r, userID)
}
