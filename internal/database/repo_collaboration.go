package database

import (
	log "unknwon.dev/clog/v2"

	"github.com/cockroachdb/errors"
	api "github.com/gogs/go-gogs-client"
	"gorm.io/gorm"
)

// Collaboration represent the relation between an individual and a repository.
type Collaboration struct {
	ID     int64      `gorm:"primary_key"`
	UserID int64      `xorm:"UNIQUE(s) INDEX NOT NULL" gorm:"uniqueIndex:collaboration_user_repo_unique;index;not null"`
	RepoID int64      `xorm:"UNIQUE(s) INDEX NOT NULL" gorm:"uniqueIndex:collaboration_user_repo_unique;index;not null"`
	Mode   AccessMode `xorm:"DEFAULT 2 NOT NULL" gorm:"not null;default:2"`
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
	collaboration := &Collaboration{
		RepoID: repoID,
		UserID: userID,
	}
	err := db.Where("repo_id = ? AND user_id = ?", repoID, userID).First(collaboration).Error
	if err != nil {
		log.Error("get collaboration [repo_id: %d, user_id: %d]: %v", repoID, userID, err)
		return false
	}
	return true
}

func (r *Repository) IsCollaborator(userID int64) bool {
	return IsCollaborator(r.ID, userID)
}

// AddCollaborator adds new collaboration to a repository with default access mode.
func (r *Repository) AddCollaborator(u *User) error {
	collaboration := &Collaboration{
		RepoID: r.ID,
		UserID: u.ID,
	}

	var existing Collaboration
	err := db.Where("repo_id = ? AND user_id = ?", r.ID, u.ID).First(&existing).Error
	if err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	collaboration.Mode = AccessModeWrite

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(collaboration).Error; err != nil {
			return err
		}
		if err := r.recalculateAccesses(tx); err != nil {
			return errors.Newf("recalculateAccesses [repo_id: %v]: %v", r.ID, err)
		}
		return nil
	})
}

func (r *Repository) getCollaborations(e *gorm.DB) ([]*Collaboration, error) {
	collaborations := make([]*Collaboration, 0)
	return collaborations, e.Where("repo_id = ?", r.ID).Find(&collaborations).Error
}

// Collaborator represents a user with collaboration details.
type Collaborator struct {
	*User
	Collaboration *Collaboration
}

func (c *Collaborator) APIFormat() *api.Collaborator {
	return &api.Collaborator{
		User: c.User.APIFormat(),
		Permissions: api.Permission{
			Admin: c.Collaboration.Mode >= AccessModeAdmin,
			Push:  c.Collaboration.Mode >= AccessModeWrite,
			Pull:  c.Collaboration.Mode >= AccessModeRead,
		},
	}
}

func (r *Repository) getCollaborators(e *gorm.DB) ([]*Collaborator, error) {
	collaborations, err := r.getCollaborations(e)
	if err != nil {
		return nil, errors.Newf("getCollaborations: %v", err)
	}

	collaborators := make([]*Collaborator, len(collaborations))
	for i, c := range collaborations {
		user, err := getUserByID(e, c.UserID)
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
	return r.getCollaborators(db)
}

// ChangeCollaborationAccessMode sets new access mode for the collaboration.
func (r *Repository) ChangeCollaborationAccessMode(userID int64, mode AccessMode) error {
	// Discard invalid input
	if mode <= AccessModeNone || mode > AccessModeOwner {
		return nil
	}

	collaboration := &Collaboration{
		RepoID: r.ID,
		UserID: userID,
	}
	err := db.Where("repo_id = ? AND user_id = ?", r.ID, userID).First(collaboration).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
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

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Collaboration{}).Where("id = ?", collaboration.ID).Updates(collaboration).Error; err != nil {
			return errors.Newf("update collaboration: %v", err)
		}

		access := &Access{
			UserID: userID,
			RepoID: r.ID,
		}
		err := tx.Where("user_id = ? AND repo_id = ?", userID, r.ID).First(access).Error
		if err == nil {
			if err := tx.Exec("UPDATE access SET mode = ? WHERE user_id = ? AND repo_id = ?", mode, userID, r.ID).Error; err != nil {
				return errors.Newf("update access table: %v", err)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			access.Mode = mode
			if err := tx.Create(access).Error; err != nil {
				return errors.Newf("insert access table: %v", err)
			}
		} else {
			return errors.Newf("get access record: %v", err)
		}

		return nil
	})
}

// DeleteCollaboration removes collaboration relation between the user and repository.
func DeleteCollaboration(repo *Repository, userID int64) (err error) {
	if !IsCollaborator(repo.ID, userID) {
		return nil
	}

	collaboration := &Collaboration{
		RepoID: repo.ID,
		UserID: userID,
	}

	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(collaboration, "repo_id = ? AND user_id = ?", repo.ID, userID)
		if result.Error != nil {
			return result.Error
		} else if result.RowsAffected == 0 {
			return nil
		}

		if err := repo.recalculateAccesses(tx); err != nil {
			return err
		}

		return nil
	})
}

func (r *Repository) DeleteCollaboration(userID int64) error {
	return DeleteCollaboration(r, userID)
}
