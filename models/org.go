// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"strings"

	"github.com/gogits/gogs/modules/base"
)

var (
	ErrOrgNotExist      = errors.New("Organization does not exist")
	ErrTeamAlreadyExist = errors.New("Team already exist")
)

// IsOrgOwner returns true if given user is in the owner team.
func (org *User) IsOrgOwner(uid int64) bool {
	return IsOrganizationOwner(org.Id, uid)
}

// IsOrgMember returns true if given user is member of organization.
func (org *User) IsOrgMember(uid int64) bool {
	return IsOrganizationMember(org.Id, uid)
}

// GetOwnerTeam returns owner team of organization.
func (org *User) GetOwnerTeam() (*Team, error) {
	t := &Team{
		OrgId: org.Id,
		Name:  OWNER_TEAM,
	}
	_, err := x.Get(t)
	return t, err
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

	// Create default owner team.
	t := &Team{
		OrgId:      org.Id,
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
	RepoIds     string `xorm:"TEXT"`
	NumMembers  int
	NumRepos    int
	Members     []*User `xorm:"-"`
}

// IsTeamMember returns true if given user is a member of team.
func (t *Team) IsMember(uid int64) bool {
	return IsTeamMember(t.OrgId, t.Id, uid)
}

// GetMembers returns all members in given team of organization.
func (t *Team) GetMembers() (err error) {
	t.Members, err = GetTeamMembers(t.OrgId, t.Id)
	return err
}

// NewTeam creates a record of new team.
// It's caller's responsibility to assign organization ID.
func NewTeam(t *Team) error {
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
	rawSql := "UPDATE `user` SET num_teams = num_teams + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, t.OrgId); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
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

// ________                ____ ___
// \_____  \_______  ____ |    |   \______ ___________
//  /   |   \_  __ \/ ___\|    |   /  ___// __ \_  __ \
// /    |    \  | \/ /_/  >    |  /\___ \\  ___/|  | \/
// \_______  /__|  \___  /|______//____  >\___  >__|
//         \/     /_____/              \/     \/

// OrgUser represents an organization-user relation.
type OrgUser struct {
	Id       int64
	Uid      int64 `xorm:"INDEX"`
	OrgId    int64 `xorm:"INDEX"`
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
