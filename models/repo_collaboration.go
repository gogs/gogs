// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"

	log "gopkg.in/clog.v1"

	api "github.com/gogs/go-gogs-client"
)

// Collaboration represent the relation between an individual and a repository.
type Collaboration struct {
	ID     int64
	RepoID int64      `xorm:"UNIQUE(s) INDEX NOT NULL"`
	UserID int64      `xorm:"UNIQUE(s) INDEX NOT NULL"`
	Mode   AccessMode `xorm:"DEFAULT 2 NOT NULL"`
}

func (c *Collaboration) ModeI18nKey() string {
	switch c.Mode {
	case ACCESS_MODE_READ:
		return "repo.settings.collaboration.read"
	case ACCESS_MODE_WRITE:
		return "repo.settings.collaboration.write"
	case ACCESS_MODE_ADMIN:
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
	has, err := x.Get(collaboration)
	if err != nil {
		log.Error(2, "get collaboration [repo_id: %d, user_id: %d]: %v", repoID, userID, err)
		return false
	}
	return has
}

func (repo *Repository) IsCollaborator(userID int64) bool {
	return IsCollaborator(repo.ID, userID)
}

// AddCollaborator adds new collaboration to a repository with default access mode.
func (repo *Repository) AddCollaborator(u *User) error {
	collaboration := &Collaboration{
		RepoID: repo.ID,
		UserID: u.ID,
	}

	has, err := x.Get(collaboration)
	if err != nil {
		return err
	} else if has {
		return nil
	}
	collaboration.Mode = ACCESS_MODE_WRITE

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(collaboration); err != nil {
		return err
	} else if err = repo.recalculateAccesses(sess); err != nil {
		return fmt.Errorf("recalculateAccesses [repo_id: %v]: %v", repo.ID, err)
	}

	return sess.Commit()
}

func (repo *Repository) getCollaborations(e Engine) ([]*Collaboration, error) {
	collaborations := make([]*Collaboration, 0)
	return collaborations, e.Find(&collaborations, &Collaboration{RepoID: repo.ID})
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
			Admin: c.Collaboration.Mode >= ACCESS_MODE_ADMIN,
			Push:  c.Collaboration.Mode >= ACCESS_MODE_WRITE,
			Pull:  c.Collaboration.Mode >= ACCESS_MODE_READ,
		},
	}
}

func (repo *Repository) getCollaborators(e Engine) ([]*Collaborator, error) {
	collaborations, err := repo.getCollaborations(e)
	if err != nil {
		return nil, fmt.Errorf("getCollaborations: %v", err)
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
func (repo *Repository) GetCollaborators() ([]*Collaborator, error) {
	return repo.getCollaborators(x)
}

// ChangeCollaborationAccessMode sets new access mode for the collaboration.
func (repo *Repository) ChangeCollaborationAccessMode(userID int64, mode AccessMode) error {
	// Discard invalid input
	if mode <= ACCESS_MODE_NONE || mode > ACCESS_MODE_OWNER {
		return nil
	}

	collaboration := &Collaboration{
		RepoID: repo.ID,
		UserID: userID,
	}
	has, err := x.Get(collaboration)
	if err != nil {
		return fmt.Errorf("get collaboration: %v", err)
	} else if !has {
		return nil
	}

	if collaboration.Mode == mode {
		return nil
	}
	collaboration.Mode = mode

	// If it's an organizational repository, merge with team access level for highest permission
	if repo.Owner.IsOrganization() {
		teams, err := GetUserTeams(repo.OwnerID, userID)
		if err != nil {
			return fmt.Errorf("GetUserTeams: [org_id: %d, user_id: %d]: %v", repo.OwnerID, userID, err)
		}
		for i := range teams {
			if mode < teams[i].Authorize {
				mode = teams[i].Authorize
			}
		}
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Id(collaboration.ID).AllCols().Update(collaboration); err != nil {
		return fmt.Errorf("update collaboration: %v", err)
	}

	access := &Access{
		UserID: userID,
		RepoID: repo.ID,
	}
	has, err = sess.Get(access)
	if err != nil {
		return fmt.Errorf("get access record: %v", err)
	}
	if has {
		_, err = sess.Exec("UPDATE access SET mode = ? WHERE user_id = ? AND repo_id = ?", mode, userID, repo.ID)
	} else {
		access.Mode = mode
		_, err = sess.Insert(access)
	}
	if err != nil {
		return fmt.Errorf("update/insert access table: %v", err)
	}

	return sess.Commit()
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

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if has, err := sess.Delete(collaboration); err != nil || has == 0 {
		return err
	} else if err = repo.recalculateAccesses(sess); err != nil {
		return err
	}

	return sess.Commit()
}

func (repo *Repository) DeleteCollaboration(userID int64) error {
	return DeleteCollaboration(repo, userID)
}
