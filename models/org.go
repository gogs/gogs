// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

type AuthorizeType int

const (
	ORG_READABLE AuthorizeType = iota + 1
	ORG_WRITABLE
	ORG_ADMIN
)

// Team represents a organization team.
type Team struct {
	Id          int64
	OrgId       int64 `xorm:"INDEX"`
	Name        string
	Description string
	Authorize   AuthorizeType
	NumMembers  int
	NumRepos    int
}

// NewTeam creates a record of new team.
func NewTeam(t *Team) error {
	_, err := x.Insert(t)
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
