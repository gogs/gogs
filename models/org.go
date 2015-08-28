// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gogits/gogs/modules/base"
)

var (
	ErrOrgNotExist      = errors.New("Organization does not exist")
	ErrTeamAlreadyExist = errors.New("Team already exist")
	ErrTeamNotExist     = errors.New("Team does not exist")
	ErrTeamNameIllegal  = errors.New("Team name contains illegal characters")
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
	ous, err := GetOrgUsersByOrgId(org.Id)
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

// IsOrgEmailUsed returns true if the e-mail has been used in organization account.
func IsOrgEmailUsed(email string) (bool, error) {
	if len(email) == 0 {
		return false, nil
	}
	return x.Get(&User{
		Email: email,
		Type:  ORGANIZATION,
	})
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

	isExist, err = IsOrgEmailUsed(org.Email)
	if err != nil {
		return err
	} else if isExist {
		return ErrEmailAlreadyUsed{org.Email}
	}

	org.LowerName = strings.ToLower(org.Name)
	org.FullName = org.Name
	org.Avatar = base.EncodeMd5(org.Email)
	org.AvatarEmail = org.Email
	// No password for organization.
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

	// Add initial creator to organization and owner team.
	ou := &OrgUser{
		Uid:      owner.Id,
		OrgID:    org.Id,
		IsOwner:  true,
		NumTeams: 1,
	}
	if _, err = sess.Insert(ou); err != nil {
		return fmt.Errorf("insert org-user relation: %v", err)
	}

	tu := &TeamUser{
		Uid:    owner.Id,
		OrgID:  org.Id,
		TeamID: t.ID,
	}
	if _, err = sess.Insert(tu); err != nil {
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
		Type:      ORGANIZATION,
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

// GetOrganizations returns given number of organizations with offset.
func GetOrganizations(num, offset int) ([]*User, error) {
	orgs := make([]*User, 0, num)
	err := x.Limit(num, offset).Where("type=1").Asc("id").Find(&orgs)
	return orgs, err
}

// TODO: need some kind of mechanism to record failure.
// DeleteOrganization completely and permanently deletes everything of organization.
func DeleteOrganization(org *User) (err error) {
	if err := DeleteUser(org); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Delete(&Team{OrgID: org.Id}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&OrgUser{OrgID: org.Id}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&TeamUser{OrgID: org.Id}); err != nil {
		sess.Rollback()
		return err
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

// GetOrgUsersByUserId returns all organization-user relations by user ID.
func GetOrgUsersByUserId(uid int64) ([]*OrgUser, error) {
	ous := make([]*OrgUser, 0, 10)
	err := x.Where("uid=?", uid).Find(&ous)
	return ous, err
}

// GetOrgUsersByOrgId returns all organization-user relations by organization ID.
func GetOrgUsersByOrgId(orgId int64) ([]*OrgUser, error) {
	ous := make([]*OrgUser, 0, 10)
	err := x.Where("org_id=?", orgId).Find(&ous)
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

// ___________
// \__    ___/___ _____    _____
//   |    |_/ __ \\__  \  /     \
//   |    |\  ___/ / __ \|  Y Y  \
//   |____| \___  >____  /__|_|  /
//              \/     \/      \/

const OWNER_TEAM = "Owners"

// Team represents a organization team.
type Team struct {
	ID          int64 `xorm:"pk autoincr"`
	OrgID       int64 `xorm:"INDEX"`
	LowerName   string
	Name        string
	Description string
	Authorize   AccessMode
	Repos       []*Repository `xorm:"-"`
	Members     []*User       `xorm:"-"`
	NumRepos    int
	NumMembers  int
}

// IsOwnerTeam returns true if team is owner team.
func (t *Team) IsOwnerTeam() bool {
	return t.Name == OWNER_TEAM
}

// IsTeamMember returns true if given user is a member of team.
func (t *Team) IsMember(uid int64) bool {
	return IsTeamMember(t.OrgID, t.ID, uid)
}

func (t *Team) getRepositories(e Engine) (err error) {
	teamRepos := make([]*TeamRepo, 0, t.NumRepos)
	if err = x.Where("team_id=?", t.ID).Find(&teamRepos); err != nil {
		return fmt.Errorf("get team-repos: %v", err)
	}

	t.Repos = make([]*Repository, 0, len(teamRepos))
	for i := range teamRepos {
		repo, err := getRepositoryByID(e, teamRepos[i].RepoID)
		if err != nil {
			return fmt.Errorf("getRepositoryById(%d): %v", teamRepos[i].RepoID, err)
		}
		t.Repos = append(t.Repos, repo)
	}
	return nil
}

// GetRepositories returns all repositories in team of organization.
func (t *Team) GetRepositories() error {
	return t.getRepositories(x)
}

func (t *Team) getMembers(e Engine) (err error) {
	t.Members, err = getTeamMembers(e, t.ID)
	return err
}

// GetMembers returns all members in team of organization.
func (t *Team) GetMembers() (err error) {
	return t.getMembers(x)
}

// AddMember adds new member to team of organization.
func (t *Team) AddMember(uid int64) error {
	return AddTeamMember(t.OrgID, t.ID, uid)
}

// RemoveMember removes member from team of organization.
func (t *Team) RemoveMember(uid int64) error {
	return RemoveTeamMember(t.OrgID, t.ID, uid)
}

func (t *Team) hasRepository(e Engine, repoID int64) bool {
	return hasTeamRepo(e, t.OrgID, t.ID, repoID)
}

// HasRepository returns true if given repository belong to team.
func (t *Team) HasRepository(repoID int64) bool {
	return t.hasRepository(x, repoID)
}

func (t *Team) addRepository(e Engine, repo *Repository) (err error) {
	if err = addTeamRepo(e, t.OrgID, t.ID, repo.ID); err != nil {
		return err
	}

	t.NumRepos++
	if _, err = e.Id(t.ID).AllCols().Update(t); err != nil {
		return fmt.Errorf("update team: %v", err)
	}

	if err = repo.recalculateTeamAccesses(e, 0); err != nil {
		return fmt.Errorf("recalculateAccesses: %v", err)
	}

	if err = t.getMembers(e); err != nil {
		return fmt.Errorf("getMembers: %v", err)
	}
	for _, u := range t.Members {
		if err = watchRepo(e, u.Id, repo.ID, true); err != nil {
			return fmt.Errorf("watchRepo: %v", err)
		}
	}
	return nil
}

// AddRepository adds new repository to team of organization.
func (t *Team) AddRepository(repo *Repository) (err error) {
	if repo.OwnerID != t.OrgID {
		return errors.New("Repository does not belong to organization")
	} else if t.HasRepository(repo.ID) {
		return nil
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = t.addRepository(sess, repo); err != nil {
		return err
	}

	return sess.Commit()
}

func (t *Team) removeRepository(e Engine, repo *Repository, recalculate bool) (err error) {
	if err = removeTeamRepo(e, t.ID, repo.ID); err != nil {
		return err
	}

	t.NumRepos--
	if _, err = e.Id(t.ID).AllCols().Update(t); err != nil {
		return err
	}

	// Don't need to recalculate when delete a repository from organization.
	if recalculate {
		if err = repo.recalculateTeamAccesses(e, t.ID); err != nil {
			return err
		}
	}

	if err = t.getMembers(e); err != nil {
		return fmt.Errorf("get team members: %v", err)
	}
	for _, u := range t.Members {
		has, err := hasAccess(e, u, repo, ACCESS_MODE_READ)
		if err != nil {
			return err
		} else if has {
			continue
		}

		if err = watchRepo(e, u.Id, repo.ID, false); err != nil {
			return err
		}
	}

	return nil
}

// RemoveRepository removes repository from team of organization.
func (t *Team) RemoveRepository(repoID int64) error {
	if !t.HasRepository(repoID) {
		return nil
	}

	repo, err := GetRepositoryByID(repoID)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = t.removeRepository(sess, repo, true); err != nil {
		return err
	}

	return sess.Commit()
}

// NewTeam creates a record of new team.
// It's caller's responsibility to assign organization ID.
func NewTeam(t *Team) (err error) {
	if err = IsUsableName(t.Name); err != nil {
		return err
	}

	has, err := x.Id(t.OrgID).Get(new(User))
	if err != nil {
		return err
	} else if !has {
		return ErrOrgNotExist
	}

	t.LowerName = strings.ToLower(t.Name)
	has, err = x.Where("org_id=?", t.OrgID).And("lower_name=?", t.LowerName).Get(new(Team))
	if err != nil {
		return err
	} else if has {
		return ErrTeamAlreadyExist
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(t); err != nil {
		sess.Rollback()
		return err
	}

	// Update organization number of teams.
	if _, err = sess.Exec("UPDATE `user` SET num_teams=num_teams+1 WHERE id = ?", t.OrgID); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

func getTeam(e Engine, orgId int64, name string) (*Team, error) {
	t := &Team{
		OrgID:     orgId,
		LowerName: strings.ToLower(name),
	}
	has, err := e.Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrTeamNotExist
	}
	return t, nil
}

// GetTeam returns team by given team name and organization.
func GetTeam(orgId int64, name string) (*Team, error) {
	return getTeam(x, orgId, name)
}

func getTeamById(e Engine, teamId int64) (*Team, error) {
	t := new(Team)
	has, err := e.Id(teamId).Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrTeamNotExist
	}
	return t, nil
}

// GetTeamById returns team by given ID.
func GetTeamById(teamId int64) (*Team, error) {
	return getTeamById(x, teamId)
}

// UpdateTeam updates information of team.
func UpdateTeam(t *Team, authChanged bool) (err error) {
	if err = IsUsableName(t.Name); err != nil {
		return err
	}

	if len(t.Description) > 255 {
		t.Description = t.Description[:255]
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	t.LowerName = strings.ToLower(t.Name)
	if _, err = sess.Id(t.ID).AllCols().Update(t); err != nil {
		return fmt.Errorf("update: %v", err)
	}

	// Update access for team members if needed.
	if authChanged {
		if err = t.getRepositories(sess); err != nil {
			return fmt.Errorf("getRepositories:%v", err)
		}

		for _, repo := range t.Repos {
			if err = repo.recalculateTeamAccesses(sess, 0); err != nil {
				return fmt.Errorf("recalculateTeamAccesses: %v", err)
			}
		}
	}

	return sess.Commit()
}

// DeleteTeam deletes given team.
// It's caller's responsibility to assign organization ID.
func DeleteTeam(t *Team) error {
	if err := t.GetRepositories(); err != nil {
		return err
	}

	// Get organization.
	org, err := GetUserByID(t.OrgID)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	// Delete all accesses.
	for _, repo := range t.Repos {
		if err = repo.recalculateTeamAccesses(sess, t.ID); err != nil {
			return err
		}
	}

	// Delete team-user.
	if _, err = sess.Where("org_id=?", org.Id).Where("team_id=?", t.ID).Delete(new(TeamUser)); err != nil {
		return err
	}

	// Delete team.
	if _, err = sess.Id(t.ID).Delete(new(Team)); err != nil {
		return err
	}
	// Update organization number of teams.
	if _, err = sess.Exec("UPDATE `user` SET num_teams=num_teams-1 WHERE id=?", t.OrgID); err != nil {
		return err
	}

	return sess.Commit()
}

// ___________                    ____ ___
// \__    ___/___ _____    _____ |    |   \______ ___________
//   |    |_/ __ \\__  \  /     \|    |   /  ___// __ \_  __ \
//   |    |\  ___/ / __ \|  Y Y  \    |  /\___ \\  ___/|  | \/
//   |____| \___  >____  /__|_|  /______//____  >\___  >__|
//              \/     \/      \/             \/     \/

// TeamUser represents an team-user relation.
type TeamUser struct {
	ID     int64 `xorm:"pk autoincr"`
	OrgID  int64 `xorm:"INDEX"`
	TeamID int64 `xorm:"UNIQUE(s)"`
	Uid    int64 `xorm:"UNIQUE(s)"`
}

func isTeamMember(e Engine, orgID, teamID, uid int64) bool {
	has, _ := e.Where("org_id=?", orgID).And("team_id=?", teamID).And("uid=?", uid).Get(new(TeamUser))
	return has
}

// IsTeamMember returns true if given user is a member of team.
func IsTeamMember(orgID, teamID, uid int64) bool {
	return isTeamMember(x, orgID, teamID, uid)
}

func getTeamMembers(e Engine, teamID int64) (_ []*User, err error) {
	teamUsers := make([]*TeamUser, 0, 10)
	if err = e.Where("team_id=?", teamID).Find(&teamUsers); err != nil {
		return nil, fmt.Errorf("get team-users: %v", err)
	}
	members := make([]*User, 0, len(teamUsers))
	for i := range teamUsers {
		member := new(User)
		if _, err = e.Id(teamUsers[i].Uid).Get(member); err != nil {
			return nil, fmt.Errorf("get user '%d': %v", teamUsers[i].Uid, err)
		}
		members = append(members, member)
	}
	return members, nil
}

// GetTeamMembers returns all members in given team of organization.
func GetTeamMembers(teamID int64) ([]*User, error) {
	return getTeamMembers(x, teamID)
}

func getUserTeams(e Engine, orgId, uid int64) ([]*Team, error) {
	tus := make([]*TeamUser, 0, 5)
	if err := e.Where("uid=?", uid).And("org_id=?", orgId).Find(&tus); err != nil {
		return nil, err
	}

	ts := make([]*Team, len(tus))
	for i, tu := range tus {
		t := new(Team)
		has, err := e.Id(tu.TeamID).Get(t)
		if err != nil {
			return nil, err
		} else if !has {
			return nil, ErrTeamNotExist
		}
		ts[i] = t
	}
	return ts, nil
}

// GetUserTeams returns all teams that user belongs to in given organization.
func GetUserTeams(orgId, uid int64) ([]*Team, error) {
	return getUserTeams(x, orgId, uid)
}

// AddTeamMember adds new member to given team of given organization.
func AddTeamMember(orgId, teamId, uid int64) error {
	if IsTeamMember(orgId, teamId, uid) {
		return nil
	}

	if err := AddOrgUser(orgId, uid); err != nil {
		return err
	}

	// Get team and its repositories.
	t, err := GetTeamById(teamId)
	if err != nil {
		return err
	}
	t.NumMembers++

	if err = t.GetRepositories(); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	tu := &TeamUser{
		Uid:    uid,
		OrgID:  orgId,
		TeamID: teamId,
	}
	if _, err = sess.Insert(tu); err != nil {
		return err
	} else if _, err = sess.Id(t.ID).Update(t); err != nil {
		return err
	}

	// Give access to team repositories.
	for _, repo := range t.Repos {
		if err = repo.recalculateTeamAccesses(sess, 0); err != nil {
			return err
		}
	}

	// We make sure it exists before.
	ou := new(OrgUser)
	if _, err = sess.Where("uid=?", uid).And("org_id=?", orgId).Get(ou); err != nil {
		return err
	}
	ou.NumTeams++
	if t.IsOwnerTeam() {
		ou.IsOwner = true
	}
	if _, err = sess.Id(ou.ID).AllCols().Update(ou); err != nil {
		return err
	}

	return sess.Commit()
}

func removeTeamMember(e Engine, orgId, teamId, uid int64) error {
	if !isTeamMember(e, orgId, teamId, uid) {
		return nil
	}

	// Get team and its repositories.
	t, err := getTeamById(e, teamId)
	if err != nil {
		return err
	}

	// Check if the user to delete is the last member in owner team.
	if t.IsOwnerTeam() && t.NumMembers == 1 {
		return ErrLastOrgOwner{UID: uid}
	}

	t.NumMembers--

	if err = t.getRepositories(e); err != nil {
		return err
	}

	// Get organization.
	org, err := getUserByID(e, orgId)
	if err != nil {
		return err
	}

	tu := &TeamUser{
		Uid:    uid,
		OrgID:  orgId,
		TeamID: teamId,
	}
	if _, err := e.Delete(tu); err != nil {
		return err
	} else if _, err = e.Id(t.ID).AllCols().Update(t); err != nil {
		return err
	}

	// Delete access to team repositories.
	for _, repo := range t.Repos {
		if err = repo.recalculateTeamAccesses(e, 0); err != nil {
			return err
		}
	}

	// This must exist.
	ou := new(OrgUser)
	_, err = e.Where("uid=?", uid).And("org_id=?", org.Id).Get(ou)
	if err != nil {
		return err
	}
	ou.NumTeams--
	if t.IsOwnerTeam() {
		ou.IsOwner = false
	}
	if _, err = e.Id(ou.ID).AllCols().Update(ou); err != nil {
		return err
	}
	return nil
}

// RemoveTeamMember removes member from given team of given organization.
func RemoveTeamMember(orgId, teamId, uid int64) error {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err := sess.Begin(); err != nil {
		return err
	}
	if err := removeTeamMember(sess, orgId, teamId, uid); err != nil {
		return err
	}
	return sess.Commit()
}

// ___________                  __________
// \__    ___/___ _____    _____\______   \ ____ ______   ____
//   |    |_/ __ \\__  \  /     \|       _// __ \\____ \ /  _ \
//   |    |\  ___/ / __ \|  Y Y  \    |   \  ___/|  |_> >  <_> )
//   |____| \___  >____  /__|_|  /____|_  /\___  >   __/ \____/
//              \/     \/      \/       \/     \/|__|

// TeamRepo represents an team-repository relation.
type TeamRepo struct {
	ID     int64 `xorm:"pk autoincr"`
	OrgID  int64 `xorm:"INDEX"`
	TeamID int64 `xorm:"UNIQUE(s)"`
	RepoID int64 `xorm:"UNIQUE(s)"`
}

func hasTeamRepo(e Engine, orgID, teamID, repoID int64) bool {
	has, _ := e.Where("org_id=?", orgID).And("team_id=?", teamID).And("repo_id=?", repoID).Get(new(TeamRepo))
	return has
}

// HasTeamRepo returns true if given repository belongs to team.
func HasTeamRepo(orgID, teamID, repoID int64) bool {
	return hasTeamRepo(x, orgID, teamID, repoID)
}

func addTeamRepo(e Engine, orgID, teamID, repoID int64) error {
	_, err := e.InsertOne(&TeamRepo{
		OrgID:  orgID,
		TeamID: teamID,
		RepoID: repoID,
	})
	return err
}

// AddTeamRepo adds new repository relation to team.
func AddTeamRepo(orgID, teamID, repoID int64) error {
	return addTeamRepo(x, orgID, teamID, repoID)
}

func removeTeamRepo(e Engine, teamID, repoID int64) error {
	_, err := e.Delete(&TeamRepo{
		TeamID: teamID,
		RepoID: repoID,
	})
	return err
}

// RemoveTeamRepo deletes repository relation to team.
func RemoveTeamRepo(teamID, repoID int64) error {
	return removeTeamRepo(x, teamID, repoID)
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
