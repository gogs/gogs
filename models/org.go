// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"strings"

	"github.com/gogits/gogs/modules/base"
)

// GetOwnerTeam returns owner team of organization.
func (org *User) GetOwnerTeam() (*Team, error) {
	t := &Team{
		OrgId: org.Id,
		Name:  OWNER_TEAM,
	}
	_, err := x.Get(t)
	return t, err
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

type AuthorizeType int

const (
	ORG_READABLE AuthorizeType = iota + 1
	ORG_WRITABLE
	ORG_ADMIN
)

const OWNER_TEAM = "Owner"

// Team represents a organization team.
type Team struct {
	Id          int64
	OrgId       int64 `xorm:"INDEX"`
	Name        string
	Description string
	Authorize   AuthorizeType
	RepoIds     string `xorm:"TEXT"`
	NumMembers  int
	NumRepos    int
}

// NewTeam creates a record of new team.
func NewTeam(t *Team) error {
	_, err := x.Insert(t)
	return err
}

func UpdateTeam(t *Team) error {
	if len(t.Description) > 255 {
		t.Description = t.Description[:255]
	}

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

func GetOrganizationCount(u *User) (int64, error) {
	return x.Where("uid=?", u.Id).Count(new(OrgUser))
}

// IsOrganizationOwner returns true if given user ID is in the owner team.
func IsOrganizationOwner(orgId, uid int64) bool {
	has, _ := x.Where("is_owner=?", true).Get(&OrgUser{Uid: uid, OrgId: orgId})
	return has
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
