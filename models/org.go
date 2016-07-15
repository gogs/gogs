// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
)

var (
	ErrOrgNotExist  = errors.New("Organization does not exist")
	ErrTeamNotExist = errors.New("Team does not exist")
)

// IsOwnedBy returns true if given user is in the owner team.
func (org *User) IsOwnedBy(uid int64) bool {
	return IsOrganizationOwner(org.Id, uid)
}

// IsOrgMember returns true if given user is member of organization.
func (org *User) IsOrgMember(uid int64) bool {
	return org.IsOrganization() && IsOrganizationMember(org.Id, uid)
}

func (org *User) getTeam(e Engine, name string) (*Team, error) {
	return getTeam(e, org.Id, name)
}

// GetTeam returns named team of organization.
func (org *User) GetTeam(name string) (*Team, error) {
	return org.getTeam(x, name)
}

func (org *User) getOwnerTeam(e Engine) (*Team, error) {
	return org.getTeam(e, OWNER_TEAM)
}

// GetOwnerTeam returns owner team of organization.
func (org *User) GetOwnerTeam() (*Team, error) {
	return org.getOwnerTeam(x)
}

func (org *User) getTeams(e Engine) error {
	return e.Where("org_id=?", org.Id).Find(&org.Teams)
}

// GetTeams returns all teams that belong to organization.
func (org *User) GetTeams() error {
	return org.getTeams(x)
}

// GetMembers returns all members of organization.
func (org *User) GetMembers() error {
	ous, err := GetOrgUsersByOrgID(org.Id)
	if err != nil {
		return err
	}

	org.Members = make([]*User, len(ous))
	for i, ou := range ous {
		org.Members[i], err = GetUserByID(ou.Uid)
		if err != nil {
			return err
		}
	}
	return nil
}

// AddMember adds new member to organization.
func (org *User) AddMember(uid int64) error {
	return AddOrgUser(org.Id, uid)
}

// RemoveMember removes member from organization.
func (org *User) RemoveMember(uid int64) error {
	return RemoveOrgUser(org.Id, uid)
}

func (org *User) removeOrgRepo(e Engine, repoID int64) error {
	return removeOrgRepo(e, org.Id, repoID)
}

// RemoveOrgRepo removes all team-repository relations of organization.
func (org *User) RemoveOrgRepo(repoID int64) error {
	return org.removeOrgRepo(x, repoID)
}

// CreateOrganization creates record of a new organization.
func CreateOrganization(org, owner *User) (err error) {
	if err = IsUsableName(org.Name); err != nil {
		return err
	}

	isExist, err := IsUserExist(0, org.Name)
	if err != nil {
		return err
	} else if isExist {
		return ErrUserAlreadyExist{org.Name}
	}

	org.LowerName = strings.ToLower(org.Name)
	org.FullName = org.Name
	org.Rands = GetUserSalt()
	org.Salt = GetUserSalt()
	org.UseCustomAvatar = true
	org.MaxRepoCreation = -1
	org.NumTeams = 1
	org.NumMembers = 1

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(org); err != nil {
		return fmt.Errorf("insert organization: %v", err)
	}
	org.GenerateRandomAvatar()

	// Add initial creator to organization and owner team.
	if _, err = sess.Insert(&OrgUser{
		Uid:      owner.Id,
		OrgID:    org.Id,
		IsOwner:  true,
		NumTeams: 1,
	}); err != nil {
		return fmt.Errorf("insert org-user relation: %v", err)
	}

	// Create default owner team.
	t := &Team{
		OrgID:      org.Id,
		LowerName:  strings.ToLower(OWNER_TEAM),
		Name:       OWNER_TEAM,
		Authorize:  ACCESS_MODE_OWNER,
		NumMembers: 1,
	}
	if _, err = sess.Insert(t); err != nil {
		return fmt.Errorf("insert owner team: %v", err)
	}

	if _, err = sess.Insert(&TeamUser{
		Uid:    owner.Id,
		OrgID:  org.Id,
		TeamID: t.ID,
	}); err != nil {
		return fmt.Errorf("insert team-user relation: %v", err)
	}

	if err = os.MkdirAll(UserPath(org.Name), os.ModePerm); err != nil {
		return fmt.Errorf("create directory: %v", err)
	}

	return sess.Commit()
}

// GetOrgByName returns organization by given name.
func GetOrgByName(name string) (*User, error) {
	if len(name) == 0 {
		return nil, ErrOrgNotExist
	}
	u := &User{
		LowerName: strings.ToLower(name),
		Type:      USER_TYPE_ORGANIZATION,
	}
	has, err := x.Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrOrgNotExist
	}
	return u, nil
}

// CountOrganizations returns number of organizations.
func CountOrganizations() int64 {
	count, _ := x.Where("type=1").Count(new(User))
	return count
}

// Organizations returns number of organizations in given page.
func Organizations(page, pageSize int) ([]*User, error) {
	orgs := make([]*User, 0, pageSize)
	return orgs, x.Limit(pageSize, (page-1)*pageSize).Where("type=1").Asc("id").Find(&orgs)
}

// DeleteOrganization completely and permanently deletes everything of organization.
func DeleteOrganization(org *User) (err error) {
	if err := DeleteUser(org); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = deleteBeans(sess,
		&Team{OrgID: org.Id},
		&OrgUser{OrgID: org.Id},
		&TeamUser{OrgID: org.Id},
	); err != nil {
		return fmt.Errorf("deleteBeans: %v", err)
	}

	if err = deleteUser(sess, org); err != nil {
		return fmt.Errorf("deleteUser: %v", err)
	}

	return sess.Commit()
}

// ________                ____ ___
// \_____  \_______  ____ |    |   \______ ___________
//  /   |   \_  __ \/ ___\|    |   /  ___// __ \_  __ \
// /    |    \  | \/ /_/  >    |  /\___ \\  ___/|  | \/
// \_______  /__|  \___  /|______//____  >\___  >__|
//         \/     /_____/              \/     \/

// OrgUser represents an organization-user relation.
type OrgUser struct {
	ID       int64 `xorm:"pk autoincr"`
	Uid      int64 `xorm:"INDEX UNIQUE(s)"`
	OrgID    int64 `xorm:"INDEX UNIQUE(s)"`
	IsPublic bool
	IsOwner  bool
	NumTeams int
}

// IsOrganizationOwner returns true if given user is in the owner team.
func IsOrganizationOwner(orgId, uid int64) bool {
	has, _ := x.Where("is_owner=?", true).And("uid=?", uid).And("org_id=?", orgId).Get(new(OrgUser))
	return has
}

// IsOrganizationMember returns true if given user is member of organization.
func IsOrganizationMember(orgId, uid int64) bool {
	has, _ := x.Where("uid=?", uid).And("org_id=?", orgId).Get(new(OrgUser))
	return has
}

// IsPublicMembership returns true if given user public his/her membership.
func IsPublicMembership(orgId, uid int64) bool {
	has, _ := x.Where("uid=?", uid).And("org_id=?", orgId).And("is_public=?", true).Get(new(OrgUser))
	return has
}

func getOrgsByUserID(sess *xorm.Session, userID int64, showAll bool) ([]*User, error) {
	orgs := make([]*User, 0, 10)
	if !showAll {
		sess.And("`org_user`.is_public=?", true)
	}
	return orgs, sess.And("`org_user`.uid=?", userID).
		Join("INNER", "`org_user`", "`org_user`.org_id=`user`.id").Find(&orgs)
}

// GetOrgsByUserID returns a list of organizations that the given user ID
// has joined.
func GetOrgsByUserID(userID int64, showAll bool) ([]*User, error) {
	return getOrgsByUserID(x.NewSession(), userID, showAll)
}

// GetOrgsByUserIDDesc returns a list of organizations that the given user ID
// has joined, ordered descending by the given condition.
func GetOrgsByUserIDDesc(userID int64, desc string, showAll bool) ([]*User, error) {
	return getOrgsByUserID(x.NewSession().Desc(desc), userID, showAll)
}

func getOwnedOrgsByUserID(sess *xorm.Session, userID int64) ([]*User, error) {
	orgs := make([]*User, 0, 10)
	return orgs, sess.Where("`org_user`.uid=?", userID).And("`org_user`.is_owner=?", true).
		Join("INNER", "`org_user`", "`org_user`.org_id=`user`.id").Find(&orgs)
}

// GetOwnedOrgsByUserID returns a list of organizations are owned by given user ID.
func GetOwnedOrgsByUserID(userID int64) ([]*User, error) {
	sess := x.NewSession()
	return getOwnedOrgsByUserID(sess, userID)
}

// GetOwnedOrganizationsByUserIDDesc returns a list of organizations are owned by
// given user ID, ordered descending by the given condition.
func GetOwnedOrgsByUserIDDesc(userID int64, desc string) ([]*User, error) {
	sess := x.NewSession()
	return getOwnedOrgsByUserID(sess.Desc(desc), userID)
}

// GetOrgUsersByUserID returns all organization-user relations by user ID.
func GetOrgUsersByUserID(uid int64, all bool) ([]*OrgUser, error) {
	ous := make([]*OrgUser, 0, 10)
	sess := x.Where("uid=?", uid)
	if !all {
		// Only show public organizations
		sess.And("is_public=?", true)
	}
	err := sess.Find(&ous)
	return ous, err
}

// GetOrgUsersByOrgID returns all organization-user relations by organization ID.
func GetOrgUsersByOrgID(orgID int64) ([]*OrgUser, error) {
	ous := make([]*OrgUser, 0, 10)
	err := x.Where("org_id=?", orgID).Find(&ous)
	return ous, err
}

// ChangeOrgUserStatus changes public or private membership status.
func ChangeOrgUserStatus(orgId, uid int64, public bool) error {
	ou := new(OrgUser)
	has, err := x.Where("uid=?", uid).And("org_id=?", orgId).Get(ou)
	if err != nil {
		return err
	} else if !has {
		return nil
	}

	ou.IsPublic = public
	_, err = x.Id(ou.ID).AllCols().Update(ou)
	return err
}

// AddOrgUser adds new user to given organization.
func AddOrgUser(orgId, uid int64) error {
	if IsOrganizationMember(orgId, uid) {
		return nil
	}

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	ou := &OrgUser{
		Uid:   uid,
		OrgID: orgId,
	}

	if _, err := sess.Insert(ou); err != nil {
		sess.Rollback()
		return err
	} else if _, err = sess.Exec("UPDATE `user` SET num_members = num_members + 1 WHERE id = ?", orgId); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

// RemoveOrgUser removes user from given organization.
func RemoveOrgUser(orgId, uid int64) error {
	ou := new(OrgUser)

	has, err := x.Where("uid=?", uid).And("org_id=?", orgId).Get(ou)
	if err != nil {
		return fmt.Errorf("get org-user: %v", err)
	} else if !has {
		return nil
	}

	u, err := GetUserByID(uid)
	if err != nil {
		return fmt.Errorf("GetUserById: %v", err)
	}
	org, err := GetUserByID(orgId)
	if err != nil {
		return fmt.Errorf("get organization: %v", err)
	} else if err = org.GetRepositories(); err != nil {
		return fmt.Errorf("GetRepositories: %v", err)
	}

	// Check if the user to delete is the last member in owner team.
	if IsOrganizationOwner(orgId, uid) {
		t, err := org.GetOwnerTeam()
		if err != nil {
			return err
		}
		if t.NumMembers == 1 {
			return ErrLastOrgOwner{UID: uid}
		}
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err := sess.Begin(); err != nil {
		return err
	}

	if _, err := sess.Id(ou.ID).Delete(ou); err != nil {
		return err
	} else if _, err = sess.Exec("UPDATE `user` SET num_members=num_members-1 WHERE id=?", orgId); err != nil {
		return err
	}

	// Delete all repository accesses.
	access := &Access{UserID: u.Id}
	for _, repo := range org.Repos {
		access.RepoID = repo.ID
		if _, err = sess.Delete(access); err != nil {
			return err
		} else if err = watchRepo(sess, u.Id, repo.ID, false); err != nil {
			return err
		}
	}

	// Delete member in his/her teams.
	teams, err := getUserTeams(sess, org.Id, u.Id)
	if err != nil {
		return err
	}
	for _, t := range teams {
		if err = removeTeamMember(sess, org.Id, t.ID, u.Id); err != nil {
			return err
		}
	}

	return sess.Commit()
}

func removeOrgRepo(e Engine, orgID, repoID int64) error {
	_, err := e.Delete(&TeamRepo{
		OrgID:  orgID,
		RepoID: repoID,
	})
	return err
}

// RemoveOrgRepo removes all team-repository relations of given organization.
func RemoveOrgRepo(orgID, repoID int64) error {
	return removeOrgRepo(x, orgID, repoID)
}

// GetUserRepositories gets all repositories of an organization,
// that the user with the given userID has access to.
func (org *User) GetUserRepositories(userID int64) (err error) {
	teams := make([]*Team, 0, org.NumTeams)
	if err = x.Sql(`SELECT team.id FROM team
INNER JOIN team_user ON team_user.team_id = team.id
WHERE team_user.org_id = ? AND team_user.uid = ?`, org.Id, userID).Find(&teams); err != nil {
		return fmt.Errorf("get teams: %v", err)
	}

	teamIDs := make([]string, len(teams))
	for i := range teams {
		teamIDs[i] = com.ToStr(teams[i].ID)
	}
	if len(teamIDs) == 0 {
		// user has no team but "IN ()" is invalid SQL
		teamIDs = append(teamIDs, "-1") // there is no repo with id=-1
	}

	repos := make([]*Repository, 0, 5)
	if err = x.Sql(fmt.Sprintf(`SELECT repository.* FROM repository
INNER JOIN team_repo ON team_repo.repo_id = repository.id
WHERE (repository.owner_id = ? AND repository.is_private = ?) OR team_repo.team_id IN (%s)
GROUP BY repository.id`, strings.Join(teamIDs, ",")), org.Id, false).Find(&repos); err != nil {
		return fmt.Errorf("get repositories: %v", err)
	}
	org.Repos = repos

	// FIXME: should I change this value inside method,
	// or only in location of caller where it's really needed?
	org.NumRepos = len(org.Repos)
	return nil
}

// GetTeams returns all teams that belong to organization,
// and that the user has joined.
func (org *User) GetUserTeams(userID int64) error {
	teams := make([]*Team, 0, 5)
	if err := x.Sql(`SELECT team.* FROM team
INNER JOIN team_user ON team_user.team_id = team.id
WHERE team_user.org_id = ? AND team_user.uid = ?`,
		org.Id, userID).Find(&teams); err != nil {
		return fmt.Errorf("get teams: %v", err)
	}

	org.Teams = teams

	// FIXME: should I change this value inside method,
	// or only in location of caller where it's really needed?
	org.NumTeams = len(org.Teams)
	return nil
}
