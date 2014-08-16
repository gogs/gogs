// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"os"
	"path"
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

// IsOrgOwner returns true if given user is in the owner team.
func (org *User) IsOrgOwner(uid int64) bool {
	return IsOrganizationOwner(org.Id, uid)
}

// IsOrgMember returns true if given user is member of organization.
func (org *User) IsOrgMember(uid int64) bool {
	return IsOrganizationMember(org.Id, uid)
}

// GetTeam returns named team of organization.
func (org *User) GetTeam(name string) (*Team, error) {
	return GetTeam(org.Id, name)
}

// GetOwnerTeam returns owner team of organization.
func (org *User) GetOwnerTeam() (*Team, error) {
	return org.GetTeam(OWNER_TEAM)
}

// GetTeams returns all teams that belong to organization.
func (org *User) GetTeams() error {
	return x.Where("org_id=?", org.Id).Find(&org.Teams)
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

	isExist, err = IsEmailUsed(org.Email)
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
		Authorize:  ORG_ADMIN,
		NumMembers: 1,
	}
	if _, err = sess.Insert(t); err != nil {
		sess.Rollback()
		return nil, err
	}

	// Add initial creator to organization and owner team.
	ou := &OrgUser{
		Uid:     owner.Id,
		OrgId:   org.Id,
		IsOwner: true,
		NumTeam: 1,
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
	NumTeam  int
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

// IsPublicMembership returns ture if given user public his/her membership.
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

	ou := &OrgUser{
		Uid:   uid,
		OrgId: orgId,
	}

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
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

	// Check if the user to delete is the last member in owner team.
	if IsOrganizationOwner(orgId, uid) {
		org, err := GetUserById(orgId)
		if err != nil {
			return err
		}
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

	return sess.Commit()
}

// ___________
// \__    ___/___ _____    _____
//   |    |_/ __ \\__  \  /     \
//   |    |\  ___/ / __ \|  Y Y  \
//   |____| \___  >____  /__|_|  /
//              \/     \/      \/

type AuthorizeType int

const (
	ORG_READABLE AuthorizeType = iota + 1
	ORG_WRITABLE
	ORG_ADMIN
)

const OWNER_TEAM = "Owners"

// Team represents a organization team.
type Team struct {
	Id          int64
	OrgId       int64 `xorm:"INDEX"`
	LowerName   string
	Name        string
	Description string
	Authorize   AuthorizeType
	RepoIds     string        `xorm:"TEXT"`
	Repos       []*Repository `xorm:"-"`
	Members     []*User       `xorm:"-"`
	NumRepos    int
	NumMembers  int
}

// IsTeamMember returns true if given user is a member of team.
func (t *Team) IsMember(uid int64) bool {
	return IsTeamMember(t.OrgId, t.Id, uid)
}

// GetRepositories returns all repositories in team of organization.
func (t *Team) GetRepositories() error {
	idStrs := strings.Split(t.RepoIds, "|")
	t.Repos = make([]*Repository, 0, len(idStrs))
	for _, str := range idStrs {
		id := com.StrTo(str).MustInt64()
		if id == 0 {
			continue
		}
		repo, err := GetRepositoryById(id)
		if err != nil {
			return err
		}
		t.Repos = append(t.Repos, repo)
	}
	return nil
}

// GetMembers returns all members in team of organization.
func (t *Team) GetMembers() (err error) {
	t.Members, err = GetTeamMembers(t.OrgId, t.Id)
	return err
}

// AddMember adds new member to team of organization.
func (t *Team) AddMember(uid int64) error {
	return AddTeamMember(t.OrgId, t.Id, uid)
}

// RemoveMember removes member from team of organization.
func (t *Team) RemoveMember(uid int64) error {
	return RemoveTeamMember(t.OrgId, t.Id, uid)
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

// GetTeam returns team by given team name and organization.
func GetTeam(orgId int64, name string) (*Team, error) {
	t := &Team{
		OrgId:     orgId,
		LowerName: strings.ToLower(name),
	}
	has, err := x.Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrTeamNotExist
	}
	return t, nil
}

// GetTeamById returns team by given ID.
func GetTeamById(teamId int64) (*Team, error) {
	t := new(Team)
	has, err := x.Id(teamId).Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrTeamNotExist
	}
	return t, nil
}

// UpdateTeam updates information of team.
func UpdateTeam(t *Team) error {
	if len(t.Description) > 255 {
		t.Description = t.Description[:255]
	}

	t.LowerName = strings.ToLower(t.Name)
	_, err := x.Id(t.Id).AllCols().Update(t)
	return err
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

// IsTeamMember returns true if given user is a member of team.
func IsTeamMember(orgId, teamId, uid int64) bool {
	has, _ := x.Where("uid=?", uid).And("org_id=?", orgId).And("team_id=?", teamId).Get(new(TeamUser))
	return has
}

// GetTeamMembers returns all members in given team of organization.
func GetTeamMembers(orgId, teamId int64) ([]*User, error) {
	tus := make([]*TeamUser, 0, 10)
	err := x.Where("org_id=?", orgId).And("team_id=?", teamId).Find(&tus)
	if err != nil {
		return nil, err
	}

	us := make([]*User, len(tus))
	for i, tu := range tus {
		us[i], err = GetUserById(tu.Uid)
		if err != nil {
			return nil, err
		}
	}
	return us, nil
}

// AddTeamMember adds new member to given team of given organization.
func AddTeamMember(orgId, teamId, uid int64) error {
	if !IsOrganizationMember(orgId, uid) || IsTeamMember(orgId, teamId, uid) {
		return nil
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

	// Get organization.
	org, err := GetUserById(orgId)
	if err != nil {
		return err
	}

	// Get user.
	u, err := GetUserById(uid)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	tu := &TeamUser{
		Uid:    uid,
		OrgId:  orgId,
		TeamId: teamId,
	}

	mode := READABLE
	if t.Authorize > ORG_READABLE {
		mode = WRITABLE
	}
	access := &Access{
		UserName: u.LowerName,
		Mode:     mode,
	}

	if _, err = sess.Insert(tu); err != nil {
		sess.Rollback()
		return err
	} else if _, err = sess.Id(t.Id).Update(t); err != nil {
		sess.Rollback()
		return err
	}

	// Give access to team repositories.
	for _, repo := range t.Repos {
		access.RepoName = path.Join(org.LowerName, repo.LowerName)
		if _, err = sess.Insert(access); err != nil {
			sess.Rollback()
			return err
		}
	}

	return sess.Commit()
}

// RemoveTeamMember removes member from given team of given organization.
func RemoveTeamMember(orgId, teamId, uid int64) error {
	if !IsTeamMember(orgId, teamId, uid) {
		return nil
	}

	// Get team and its repositories.
	t, err := GetTeamById(teamId)
	if err != nil {
		return err
	}
	t.NumMembers--

	if err = t.GetRepositories(); err != nil {
		return err
	}

	// Get organization.
	org, err := GetUserById(orgId)
	if err != nil {
		return err
	}

	// Get user.
	u, err := GetUserById(uid)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	tu := &TeamUser{
		Uid:    uid,
		OrgId:  orgId,
		TeamId: teamId,
	}

	access := &Access{
		UserName: u.LowerName,
	}

	if _, err := sess.Delete(tu); err != nil {
		sess.Rollback()
		return err
	} else if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
		sess.Rollback()
		return err
	}

	// Delete access to team repositories.
	for _, repo := range t.Repos {
		access.RepoName = path.Join(org.LowerName, repo.LowerName)
		if _, err = sess.Delete(access); err != nil {
			sess.Rollback()
			return err
		}
	}

	return sess.Commit()
}
