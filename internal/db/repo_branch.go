// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogs/git-module"
	"github.com/unknwon/com"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/tool"
)

type Branch struct {
	RepoPath string
	Name     string

	IsProtected bool
	Commit      *git.Commit
}

func GetBranchesByPath(path string) ([]*Branch, error) {
	gitRepo, err := git.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open repository: %v", err)
	}

	names, err := gitRepo.Branches()
	if err != nil {
		return nil, fmt.Errorf("list branches")
	}

	branches := make([]*Branch, len(names))
	for i := range names {
		branches[i] = &Branch{
			RepoPath: path,
			Name:     names[i],
		}
	}
	return branches, nil
}

var _ errutil.NotFound = (*ErrBranchNotExist)(nil)

type ErrBranchNotExist struct {
	args map[string]any
}

func IsErrBranchNotExist(err error) bool {
	_, ok := err.(ErrBranchNotExist)
	return ok
}

func (err ErrBranchNotExist) Error() string {
	return fmt.Sprintf("branch does not exist: %v", err.args)
}

func (ErrBranchNotExist) NotFound() bool {
	return true
}

func (repo *Repository) GetBranch(name string) (*Branch, error) {
	if !git.RepoHasBranch(repo.RepoPath(), name) {
		return nil, ErrBranchNotExist{args: map[string]any{"name": name}}
	}
	return &Branch{
		RepoPath: repo.RepoPath(),
		Name:     name,
	}, nil
}

func (repo *Repository) GetBranches() ([]*Branch, error) {
	return GetBranchesByPath(repo.RepoPath())
}

func (br *Branch) GetCommit() (*git.Commit, error) {
	gitRepo, err := git.Open(br.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("open repository: %v", err)
	}
	return gitRepo.BranchCommit(br.Name)
}

type ProtectBranchWhitelist struct {
	ID              int64
	ProtectBranchID int64
	RepoID          int64  `xorm:"UNIQUE(protect_branch_whitelist)"`
	Name            string `xorm:"UNIQUE(protect_branch_whitelist)"`
	UserID          int64  `xorm:"UNIQUE(protect_branch_whitelist)"`
}

// IsUserInProtectBranchWhitelist returns true if given user is in the whitelist of a branch in a repository.
func IsUserInProtectBranchWhitelist(repoID, userID int64, branch string) bool {
	has, err := x.Where("repo_id = ?", repoID).And("user_id = ?", userID).And("name = ?", branch).Get(new(ProtectBranchWhitelist))
	return has && err == nil
}

// ProtectBranch contains options of a protected branch.
type ProtectBranch struct {
	ID                 int64
	RepoID             int64  `xorm:"UNIQUE(protect_branch)"`
	Name               string `xorm:"UNIQUE(protect_branch)"`
	Protected          bool
	RequirePullRequest bool
	EnableWhitelist    bool
	WhitelistUserIDs   string `xorm:"TEXT"`
	WhitelistTeamIDs   string `xorm:"TEXT"`
}

// GetProtectBranchOfRepoByName returns *ProtectBranch by branch name in given repository.
func GetProtectBranchOfRepoByName(repoID int64, name string) (*ProtectBranch, error) {
	protectBranch := &ProtectBranch{
		RepoID: repoID,
		Name:   name,
	}
	has, err := x.Get(protectBranch)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrBranchNotExist{args: map[string]any{"name": name}}
	}
	return protectBranch, nil
}

// IsBranchOfRepoRequirePullRequest returns true if branch requires pull request in given repository.
func IsBranchOfRepoRequirePullRequest(repoID int64, name string) bool {
	protectBranch, err := GetProtectBranchOfRepoByName(repoID, name)
	if err != nil {
		return false
	}
	return protectBranch.Protected && protectBranch.RequirePullRequest
}

// UpdateProtectBranch saves branch protection options.
// If ID is 0, it creates a new record. Otherwise, updates existing record.
func UpdateProtectBranch(protectBranch *ProtectBranch) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if protectBranch.ID == 0 {
		if _, err = sess.Insert(protectBranch); err != nil {
			return fmt.Errorf("Insert: %v", err)
		}
	}

	if _, err = sess.ID(protectBranch.ID).AllCols().Update(protectBranch); err != nil {
		return fmt.Errorf("Update: %v", err)
	}

	return sess.Commit()
}

// UpdateOrgProtectBranch saves branch protection options of organizational repository.
// If ID is 0, it creates a new record. Otherwise, updates existing record.
// This function also performs check if whitelist user and team's IDs have been changed
// to avoid unnecessary whitelist delete and regenerate.
func UpdateOrgProtectBranch(repo *Repository, protectBranch *ProtectBranch, whitelistUserIDs, whitelistTeamIDs string) (err error) {
	if err = repo.GetOwner(); err != nil {
		return fmt.Errorf("GetOwner: %v", err)
	} else if !repo.Owner.IsOrganization() {
		return fmt.Errorf("expect repository owner to be an organization")
	}

	hasUsersChanged := false
	validUserIDs := tool.StringsToInt64s(strings.Split(protectBranch.WhitelistUserIDs, ","))
	if protectBranch.WhitelistUserIDs != whitelistUserIDs {
		hasUsersChanged = true
		userIDs := tool.StringsToInt64s(strings.Split(whitelistUserIDs, ","))
		validUserIDs = make([]int64, 0, len(userIDs))
		for _, userID := range userIDs {
			if !Perms.Authorize(context.TODO(), userID, repo.ID, AccessModeWrite,
				AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			) {
				continue // Drop invalid user ID
			}

			validUserIDs = append(validUserIDs, userID)
		}

		protectBranch.WhitelistUserIDs = strings.Join(tool.Int64sToStrings(validUserIDs), ",")
	}

	hasTeamsChanged := false
	validTeamIDs := tool.StringsToInt64s(strings.Split(protectBranch.WhitelistTeamIDs, ","))
	if protectBranch.WhitelistTeamIDs != whitelistTeamIDs {
		hasTeamsChanged = true
		teamIDs := tool.StringsToInt64s(strings.Split(whitelistTeamIDs, ","))
		teams, err := GetTeamsHaveAccessToRepo(repo.OwnerID, repo.ID, AccessModeWrite)
		if err != nil {
			return fmt.Errorf("GetTeamsHaveAccessToRepo [org_id: %d, repo_id: %d]: %v", repo.OwnerID, repo.ID, err)
		}
		validTeamIDs = make([]int64, 0, len(teams))
		for i := range teams {
			if teams[i].HasWriteAccess() && com.IsSliceContainsInt64(teamIDs, teams[i].ID) {
				validTeamIDs = append(validTeamIDs, teams[i].ID)
			}
		}

		protectBranch.WhitelistTeamIDs = strings.Join(tool.Int64sToStrings(validTeamIDs), ",")
	}

	// Make sure protectBranch.ID is not 0 for whitelists
	if protectBranch.ID == 0 {
		if _, err = x.Insert(protectBranch); err != nil {
			return fmt.Errorf("Insert: %v", err)
		}
	}

	// Merge users and members of teams
	var whitelists []*ProtectBranchWhitelist
	if hasUsersChanged || hasTeamsChanged {
		mergedUserIDs := make(map[int64]bool)
		for _, userID := range validUserIDs {
			// Empty whitelist users can cause an ID with 0
			if userID != 0 {
				mergedUserIDs[userID] = true
			}
		}

		for _, teamID := range validTeamIDs {
			members, err := GetTeamMembers(teamID)
			if err != nil {
				return fmt.Errorf("GetTeamMembers [team_id: %d]: %v", teamID, err)
			}

			for i := range members {
				mergedUserIDs[members[i].ID] = true
			}
		}

		whitelists = make([]*ProtectBranchWhitelist, 0, len(mergedUserIDs))
		for userID := range mergedUserIDs {
			whitelists = append(whitelists, &ProtectBranchWhitelist{
				ProtectBranchID: protectBranch.ID,
				RepoID:          repo.ID,
				Name:            protectBranch.Name,
				UserID:          userID,
			})
		}
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.ID(protectBranch.ID).AllCols().Update(protectBranch); err != nil {
		return fmt.Errorf("Update: %v", err)
	}

	// Refresh whitelists
	if hasUsersChanged || hasTeamsChanged {
		if _, err = sess.Delete(&ProtectBranchWhitelist{ProtectBranchID: protectBranch.ID}); err != nil {
			return fmt.Errorf("delete old protect branch whitelists: %v", err)
		} else if _, err = sess.Insert(whitelists); err != nil {
			return fmt.Errorf("insert new protect branch whitelists: %v", err)
		}
	}

	return sess.Commit()
}

// GetProtectBranchesByRepoID returns a list of *ProtectBranch in given repository.
func GetProtectBranchesByRepoID(repoID int64) ([]*ProtectBranch, error) {
	protectBranches := make([]*ProtectBranch, 0, 2)
	return protectBranches, x.Where("repo_id = ? and protected = ?", repoID, true).Asc("name").Find(&protectBranches)
}
