// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"os"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/modules/base"
)

var (
	ErrOrgNotExist      = errors.New("Organization does not exist")
	ErrTeamAlreadyExist = errors.New("Team already exist")
	ErrTeamNotExist     = errors.New("Team does not exist")
	ErrTeamNameIllegal  = errors.New("Team name contains illegal characters")
	ErrLastOrgOwner     = errors.New("The user to remove is the last member in owner team")
)

// IsOwnedBy returns true if given user is in the owner team.
func (org *User) IsOwnedBy(uid int64) bool {
	return IsOrganizationOwner(org.Id, uid)
}

// IsOrgMember returns true if given user is member of organization.
func (org *User) IsOrgMember(uid int64) bool {
	return IsOrganizationMember(org.Id, uid)
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
		org.Members[i], err = GetUserById(ou.Uid)
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
func CreateOrganization(org, owner *User) (*User, error) {
	if !IsLegalName(org.Name) {
		return nil, ErrUserNameIllegal
	}

	isExist, err := IsUserExist(org.Name)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrUserAlreadyExist
	}

	isExist, err = IsOrgEmailUsed(org.Email)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrEmailAlreadyUsed
	}

	org.LowerName = strings.ToLower(org.Name)
	org.FullName = org.Name
	org.Avatar = base.EncodeMd5(org.Email)
	org.AvatarEmail = org.Email
	// No password for organization.
	org.NumTeams = 1
	org.NumMembers = 1

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if _, err = sess.Insert(org); err != nil {
		sess.Rollback()
		return nil, err
	}

	if err = os.MkdirAll(UserPath(org.Name), os.ModePerm); err != nil {
		sess.Rollback()
		return nil, err
	}

	// Create default owner team.
	t := &Team{
		OrgId:      org.Id,
		LowerName:  strings.ToLower(OWNER_TEAM),
		Name:       OWNER_TEAM,
		Authorize:  ACCESS_MODE_OWNER,
		NumMembers: 1,
	}
	if _, err = sess.Insert(t); err != nil {
		sess.Rollback()
		return nil, err
	}

	// Add initial creator to organization and owner team.
	ou := &OrgUser{
		Uid:      owner.Id,
		OrgId:    org.Id,
		IsOwner:  true,
		NumTeams: 1,
	}
	if _, err = sess.Insert(ou); err != nil {
		sess.Rollback()
		return nil, err
	}

	tu := &TeamUser{
		Uid:    owner.Id,
		OrgId:  org.Id,
		TeamId: t.Id,
	}
	if _, err = sess.Insert(tu); err != nil {
		sess.Rollback()
		return nil, err
	}

	return org, sess.Commit()
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

	if _, err = sess.Delete(&Team{OrgId: org.Id}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&OrgUser{OrgId: org.Id}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&TeamUser{OrgId: org.Id}); err != nil {
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
	Id       int64
	Uid      int64 `xorm:"INDEX UNIQUE(s)"`
	OrgId    int64 `xorm:"INDEX UNIQUE(s)"`
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
	_, err = x.Id(ou.Id).AllCols().Update(ou)
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
		OrgId: orgId,
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
		return err
	} else if !has {
		return nil
	}

	u, err := GetUserById(uid)
	if err != nil {
		return err
	}
	org, err := GetUserById(orgId)
	if err != nil {
		return err
	}

	// Check if the user to delete is the last member in owner team.
	if IsOrganizationOwner(orgId, uid) {
		t, err := org.GetOwnerTeam()
		if err != nil {
			return err
		}
		if t.NumMembers == 1 {
			return ErrLastOrgOwner
		}
	}

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	if _, err := sess.Id(ou.Id).Delete(ou); err != nil {
		sess.Rollback()
		return err
	} else if _, err = sess.Exec("UPDATE `user` SET num_members = num_members - 1 WHERE id = ?", orgId); err != nil {
		sess.Rollback()
		return err
	}

	// Delete all repository accesses.
	if err = org.GetRepositories(); err != nil {
		sess.Rollback()
		return err
	}
	access := &Access{
		UserID: u.Id,
	}
	for _, repo := range org.Repos {
		access.RepoID = repo.Id
		if _, err = sess.Delete(access); err != nil {
			sess.Rollback()
			return err
		} else if err = WatchRepo(u.Id, repo.Id, false); err != nil {
			sess.Rollback()
			return err
		}
	}

	// Delete member in his/her teams.
	ts, err := GetUserTeams(org.Id, u.Id)
	if err != nil {
		return err
	}
	for _, t := range ts {
		if err = removeTeamMember(sess, org.Id, t.Id, u.Id); err != nil {
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
	Id          int64
	OrgId       int64 `xorm:"INDEX"`
	LowerName   string
	Name        string
	Description string
	Authorize   AccessMode
	RepoIds     string        `xorm:"TEXT"`
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
	return IsTeamMember(t.OrgId, t.Id, uid)
}

func (t *Team) getRepositories(e Engine) error {
	idStrs := strings.Split(t.RepoIds, "|")
	t.Repos = make([]*Repository, 0, len(idStrs))
	for _, str := range idStrs {
		if len(str) == 0 {
			continue
		}
		id := com.StrTo(str[1:]).MustInt64()
		if id == 0 {
			continue
		}
		repo, err := getRepositoryById(e, id)
		if err != nil {
			return err
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
	t.Members, err = getTeamMembers(e, t.Id)
	return err
}

// GetMembers returns all members in team of organization.
func (t *Team) GetMembers() (err error) {
	return t.getMembers(x)
}

// AddMember adds new member to team of organization.
func (t *Team) AddMember(uid int64) error {
	return AddTeamMember(t.OrgId, t.Id, uid)
}

// RemoveMember removes member from team of organization.
func (t *Team) RemoveMember(uid int64) error {
	return RemoveTeamMember(t.OrgId, t.Id, uid)
}

// AddRepository adds new repository to team of organization.
func (t *Team) AddRepository(repo *Repository) (err error) {
	idStr := "$" + com.ToStr(repo.Id) + "|"
	if repo.OwnerId != t.OrgId {
		return errors.New("Repository not belong to organization")
	} else if strings.Contains(t.RepoIds, idStr) {
		return nil
	}

	if err = repo.GetOwner(); err != nil {
		return err
	} else if err = t.GetMembers(); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	t.NumRepos++
	t.RepoIds += idStr
	if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
		return err
	}

	if err = repo.recalculateAccesses(sess); err != nil {
		return err
	}

	for _, u := range t.Members {
		if err = watchRepo(sess, u.Id, repo.Id, true); err != nil {
			return err
		}
	}
	return sess.Commit()
}

func (t *Team) HasRepository(repo *Repository) bool {
	idStr := "$" + com.ToStr(repo.Id) + "|"
	return strings.Contains(t.RepoIds, idStr)
}

// RemoveRepository removes repository from team of organization.
func (t *Team) RemoveRepository(repoId int64) error {
	idStr := "$" + com.ToStr(repoId) + "|"
	if !strings.Contains(t.RepoIds, idStr) {
		return nil
	}

	repo, err := GetRepositoryById(repoId)
	if err != nil {
		return err
	}

	if err = repo.GetOwner(); err != nil {
		return err
	} else if err = t.GetMembers(); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	t.NumRepos--
	t.RepoIds = strings.Replace(t.RepoIds, idStr, "", 1)
	if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
		return err
	}

	if err = repo.recalculateAccesses(sess); err != nil {
		return err
	}

	for _, u := range t.Members {
		if err = watchRepo(sess, u.Id, repo.Id, false); err != nil {
			return err
		}
	}

	return sess.Commit()
}

// NewTeam creates a record of new team.
// It's caller's responsibility to assign organization ID.
func NewTeam(t *Team) error {
	if !IsLegalName(t.Name) {
		return ErrTeamNameIllegal
	}

	has, err := x.Id(t.OrgId).Get(new(User))
	if err != nil {
		return err
	} else if !has {
		return ErrOrgNotExist
	}

	t.LowerName = strings.ToLower(t.Name)
	has, err = x.Where("org_id=?", t.OrgId).And("lower_name=?", t.LowerName).Get(new(Team))
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
	if _, err = sess.Exec("UPDATE `user` SET num_teams = num_teams + 1 WHERE id = ?", t.OrgId); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

func getTeam(e Engine, orgId int64, name string) (*Team, error) {
	t := &Team{
		OrgId:     orgId,
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
	if !IsLegalName(t.Name) {
		return ErrTeamNameIllegal
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
	if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
		return err
	}

	// Update access for team members if needed.
	if authChanged {
		if err = t.getRepositories(sess); err != nil {
			return err
		}

		for _, repo := range t.Repos {
			if err = repo.recalculateAccesses(sess); err != nil {
				return err
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
	} else if err = t.GetMembers(); err != nil {
		return err
	}

	// Get organization.
	org, err := GetUserById(t.OrgId)
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
		if err = repo.recalculateAccesses(sess); err != nil {
			return err
		}
	}

	// Delete team-user.
	if _, err = sess.Where("org_id=?", org.Id).Where("team_id=?", t.Id).Delete(new(TeamUser)); err != nil {
		return err
	}

	// Delete team.
	if _, err = sess.Id(t.Id).Delete(new(Team)); err != nil {
		return err
	}
	// Update organization number of teams.
	if _, err = sess.Exec("UPDATE `user` SET num_teams = num_teams - 1 WHERE id = ?", t.OrgId); err != nil {
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
	Id     int64
	Uid    int64
	OrgId  int64 `xorm:"INDEX"`
	TeamId int64
}

func isTeamMember(e Engine, orgId, teamId, uid int64) bool {
	has, _ := e.Where("uid=?", uid).And("org_id=?", orgId).And("team_id=?", teamId).Get(new(TeamUser))
	return has
}

// IsTeamMember returns true if given user is a member of team.
func IsTeamMember(orgId, teamId, uid int64) bool {
	return isTeamMember(x, orgId, teamId, uid)
}

func getTeamMembers(e Engine, teamID int64) ([]*User, error) {
	us := make([]*User, 0, 10)
	err := e.Sql("SELECT * FROM `user` JOIN `team_user` ON `team_user`.`team_id` = ? AND `team_user`.`uid` = `user`.`id`", teamID).Find(&us)
	return us, err
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
		has, err := e.Id(tu.TeamId).Get(t)
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
		OrgId:  orgId,
		TeamId: teamId,
	}

	if _, err = sess.Insert(tu); err != nil {
		return err
	} else if _, err = sess.Id(t.Id).Update(t); err != nil {
		return err
	}

	// Give access to team repositories.
	for _, repo := range t.Repos {
		if err = repo.recalculateAccesses(sess); err != nil {
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
	if _, err = sess.Id(ou.Id).AllCols().Update(ou); err != nil {
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
		return ErrLastOrgOwner
	}

	t.NumMembers--

	if err = t.getRepositories(e); err != nil {
		return err
	}

	// Get organization.
	org, err := getUserById(e, orgId)
	if err != nil {
		return err
	}

	tu := &TeamUser{
		Uid:    uid,
		OrgId:  orgId,
		TeamId: teamId,
	}

	if _, err := e.Delete(tu); err != nil {
		return err
	} else if _, err = e.Id(t.Id).AllCols().Update(t); err != nil {
		return err
	}

	// Delete access to team repositories.
	for _, repo := range t.Repos {
		if err = repo.recalculateAccesses(e); err != nil {
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
	if _, err = e.Id(ou.Id).AllCols().Update(ou); err != nil {
		return err
	}
	return nil
}

// RemoveTeamMember removes member from given team of given organization.
func RemoveTeamMember(orgId, teamId, uid int64) error {
	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	if err := removeTeamMember(sess, orgId, teamId, uid); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}
