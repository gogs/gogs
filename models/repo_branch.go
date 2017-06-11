// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"strings"

	"github.com/Unknwon/com"
	"github.com/gogits/git-module"

	"github.com/gogits/gogs/pkg/tool"
)

type Branch struct {
	RepoPath string
	Name     string

	IsProtected bool
	Commit      *git.Commit
}

func GetBranchesByPath(path string) ([]*Branch, error) {
	gitRepo, err := git.OpenRepository(path)
	if err != nil {
		return nil, err
	}

	brs, err := gitRepo.GetBranches()
	if err != nil {
		return nil, err
	}

	branches := make([]*Branch, len(brs))
	for i := range brs {
		branches[i] = &Branch{
			RepoPath: path,
			Name:     brs[i],
		}
	}
	return branches, nil
}

func (repo *Repository) GetBranch(br string) (*Branch, error) {
	if !git.IsBranchExist(repo.RepoPath(), br) {
		return nil, ErrBranchNotExist{br}
	}
	return &Branch{
		RepoPath: repo.RepoPath(),
		Name:     br,
	}, nil
}

func (repo *Repository) GetBranches() ([]*Branch, error) {
	return GetBranchesByPath(repo.RepoPath())
}

func (br *Branch) GetCommit() (*git.Commit, error) {
	gitRepo, err := git.OpenRepository(br.RepoPath)
	if err != nil {
		return nil, err
	}
	return gitRepo.GetBranchCommit(br.Name)
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

// GetProtectBranchOfRepoByName returns *ProtectBranch by branch name in given repostiory.
func GetProtectBranchOfRepoByName(repoID int64, name string) (*ProtectBranch, error) {
	protectBranch := &ProtectBranch{
		RepoID: repoID,
		Name:   name,
	}
	has, err := x.Get(protectBranch)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrBranchNotExist{name}
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

	if _, err = sess.Id(protectBranch.ID).AllCols().Update(protectBranch); err != nil {
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
			has, err := HasAccess(userID, repo, ACCESS_MODE_WRITE)
			if err != nil {
				return fmt.Errorf("HasAccess [user_id: %d, repo_id: %d]: %v", userID, protectBranch.RepoID, err)
			} else if !has {
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
		teams, err := GetTeamsHaveAccessToRepo(repo.OwnerID, repo.ID, ACCESS_MODE_WRITE)
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

	if _, err = sess.Id(protectBranch.ID).AllCols().Update(protectBranch); err != nil {
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

// GetProtectBranchesByRepoID returns a list of *ProtectBranch in given repostiory.
func GetProtectBranchesByRepoID(repoID int64) ([]*ProtectBranch, error) {
	protectBranches := make([]*ProtectBranch, 0, 2)
	return protectBranches, x.Where("repo_id = ?", repoID).Asc("name").Find(&protectBranches)
}
