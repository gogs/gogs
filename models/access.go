// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

//import (
//	"github.com/go-xorm/xorm"
//)

type AccessMode int

const (
	NoAccess AccessMode = iota
	ReadAccess
	WriteAccess
	AdminAccess
	OwnerAccess
)

func maxAccessMode(modes ...AccessMode) AccessMode {
	max := NoAccess
	for _, mode := range modes {
		if mode > max {
			max = mode
		}
	}
	return max
}

// Access represents the highest access level of a user to the repository. The only access type
// that is not in this table is the real owner of a repository. In case of an organization
// repository, the members of the owners team are in this table.
type Access struct {
	ID     int64 `xorm:"pk autoincr"`
	UserID int64 `xorm:"UNIQUE(s)"`
	RepoID int64 `xorm:"UNIQUE(s)"`
	Mode   AccessMode
}

// HasAccess returns true if someone has the request access level. User can be nil!
func HasAccess(u *User, r *Repository, testMode AccessMode) (bool, error) {
	mode, err := AccessLevel(u, r)
	return testMode <= mode, err
}

// Return the Access a user has to a repository. Will return NoneAccess if the
// user does not have access. User can be nil!
func AccessLevel(u *User, r *Repository) (AccessMode, error) {
	mode := NoAccess
	if !r.IsPrivate {
		mode = ReadAccess
	}

	if u != nil {
		if u.Id == r.OwnerId {
			return OwnerAccess, nil
		}

		a := &Access{UserID: u.Id, RepoID: r.Id}
		if has, err := x.Get(a); !has || err != nil {
			return mode, err
		}
		return a.Mode, nil
	}

	return mode, nil
}

// GetAccessibleRepositories finds all repositories where a user has access to,
// besides his own.
func (u *User) GetAccessibleRepositories() (map[*Repository]AccessMode, error) {
	accesses := make([]*Access, 0, 10)
	if err := x.Find(&accesses, &Access{UserID: u.Id}); err != nil {
		return nil, err
	}

	repos := make(map[*Repository]AccessMode, len(accesses))
	for _, access := range accesses {
		repo, err := GetRepositoryById(access.RepoID)
		if err != nil {
			return nil, err
		}
		err = repo.GetOwner()
		if err != nil {
			return nil, err
		}
		repos[repo] = access.Mode
	}

	return repos, nil
}

// Recalculate all accesses for repository
func (r *Repository) RecalcAccessSess() error {
	accessMap := make(map[int64]AccessMode, 20)

	// Give all collaborators write access
	collaborators, err := r.GetCollaborators()
	if err != nil {
		return err
	}
	for _, c := range collaborators {
		accessMap[c.Id] = WriteAccess
	}

	if err := r.GetOwner(); err != nil {
		return err
	}
	if r.Owner.IsOrganization() {
		if err = r.Owner.GetTeams(); err != nil {
			return err
		}

		for _, team := range r.Owner.Teams {
			if !(team.IsOwnerTeam() || team.HasRepository(r)) {
				continue
			}

			if err = team.GetMembers(); err != nil {
				return err
			}
			for _, u := range team.Members {
				accessMap[u.Id] = maxAccessMode(accessMap[u.Id], team.Authorize)
			}
		}
	}

	minMode := ReadAccess
	if !r.IsPrivate {
		minMode = WriteAccess
	}

	newAccesses := make([]Access, 0, len(accessMap))
	for userID, mode := range accessMap {
		if userID == r.OwnerId || mode <= minMode {
			continue
		}
		newAccesses = append(newAccesses, Access{UserID: userID, RepoID: r.Id, Mode: mode})
	}

	// Delete old accesses for repository
	if _, err = x.Delete(&Access{RepoID: r.Id}); err != nil {
		return err
	}

	// And insert the new ones
	if _, err = x.Insert(newAccesses); err != nil {
		return err
	}

	return nil

}
