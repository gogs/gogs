// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

type AccessMode int

const (
	ACCESS_MODE_NONE AccessMode = iota
	ACCESS_MODE_READ
	ACCESS_MODE_WRITE
	ACCESS_MODE_ADMIN
	ACCESS_MODE_OWNER
)

func maxAccessMode(modes ...AccessMode) AccessMode {
	max := ACCESS_MODE_NONE
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
	mode := ACCESS_MODE_NONE
	if !r.IsPrivate {
		mode = ACCESS_MODE_READ
	}

	if u != nil {
		if u.Id == r.OwnerId {
			return ACCESS_MODE_OWNER, nil
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
		if err = repo.GetOwner(); err != nil {
			return nil, err
		} else if repo.OwnerId == u.Id {
			continue
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
		accessMap[c.Id] = ACCESS_MODE_WRITE
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

	minMode := ACCESS_MODE_READ
	if !r.IsPrivate {
		minMode = ACCESS_MODE_WRITE
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
