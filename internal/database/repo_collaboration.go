package database

import (
	"gogs.io/gogs/internal/api/v1types"
)

// Collaboration represents the relation between an individual and a repository.
type Collaboration struct {
	ID     int64      `gorm:"primaryKey"`
	UserID int64      `gorm:"uniqueIndex:UQE_collaboration_s;index:IDX_collaboration_user_id;not null"`
	RepoID int64      `gorm:"uniqueIndex:UQE_collaboration_s;index:IDX_collaboration_repo_id;not null"`
	Mode   AccessMode `gorm:"not null;default:2"`
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

// Collaborator represents a user with collaboration details.
type Collaborator struct {
	*User
	Collaboration *Collaboration
}

func (c *Collaborator) APIFormat() *v1types.RepositoryCollaborator {
	return &v1types.RepositoryCollaborator{
		User: c.User.APIFormat(),
		Permissions: v1types.RepositoryPermission{
			Admin: c.Collaboration.Mode >= AccessModeAdmin,
			Push:  c.Collaboration.Mode >= AccessModeWrite,
			Pull:  c.Collaboration.Mode >= AccessModeRead,
		},
	}
}
