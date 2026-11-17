package database

import (
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
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
