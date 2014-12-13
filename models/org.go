// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"

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
		Authorize:  ORG_ADMIN,
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
		UserName: u.LowerName,
	}
	for _, repo := range org.Repos {
		access.RepoName = path.Join(org.LowerName, repo.LowerName)
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
		if err = removeTeamMemberWithSess(org.Id, t.Id, u.Id, sess); err != nil {
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

type AuthorizeType int

const (
	ORG_READABLE AuthorizeType = iota + 1
	ORG_WRITABLE
	ORG_ADMIN
)

func AuthorizeToAccessType(auth AuthorizeType) AccessType {
	if auth == ORG_READABLE {
		return READABLE
	}
	return WRITABLE
}

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

// IsOwnerTeam returns true if team is owner team.
func (t *Team) IsOwnerTeam() bool {
	return t.Name == OWNER_TEAM
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
		if len(str) == 0 {
			continue
		}
		id := com.StrTo(str[1:]).MustInt64()
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

// addAccessWithAuthorize inserts or updates access with given mode.
func addAccessWithAuthorize(sess *xorm.Session, access *Access, mode AccessType) error {
	has, err := x.Get(access)
	if err != nil {
		return fmt.Errorf("fail to get access: %v", err)
	}
	access.Mode = mode
	if has {
		if _, err = sess.Id(access.Id).Update(access); err != nil {
			return fmt.Errorf("fail to update access: %v", err)
		}
	} else {
		if _, err = sess.Insert(access); err != nil {
			return fmt.Errorf("fail to insert access: %v", err)
		}
	}
	return nil
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
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	t.NumRepos++
	t.RepoIds += idStr
	if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
		sess.Rollback()
		return err
	}

	// Give access to team members.
	mode := AuthorizeToAccessType(t.Authorize)

	for _, u := range t.Members {
		auth, err := GetHighestAuthorize(t.OrgId, u.Id, repo.Id, t.Id)
		if err != nil {
			sess.Rollback()
			return err
		}

		access := &Access{
			UserName: u.LowerName,
			RepoName: path.Join(repo.Owner.LowerName, repo.LowerName),
		}
		if auth < t.Authorize {
			if err = addAccessWithAuthorize(sess, access, mode); err != nil {
				sess.Rollback()
				return err
			}
		}
		if err = WatchRepo(u.Id, repo.Id, true); err != nil {
			sess.Rollback()
			return err
		}
	}
	return sess.Commit()
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
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	t.NumRepos--
	t.RepoIds = strings.Replace(t.RepoIds, idStr, "", 1)
	if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
		sess.Rollback()
		return err
	}

	// Remove access to team members.
	for _, u := range t.Members {
		auth, err := GetHighestAuthorize(t.OrgId, u.Id, repo.Id, t.Id)
		if err != nil {
			sess.Rollback()
			return err
		}

		access := &Access{
			UserName: u.LowerName,
			RepoName: path.Join(repo.Owner.LowerName, repo.LowerName),
		}
		if auth == 0 {
			if _, err = sess.Delete(access); err != nil {
				sess.Rollback()
				return fmt.Errorf("fail to delete access: %v", err)
			} else if err = WatchRepo(u.Id, repo.Id, false); err != nil {
				sess.Rollback()
				return err
			}
		} else if auth < t.Authorize {
			if err = addAccessWithAuthorize(sess, access, AuthorizeToAccessType(auth)); err != nil {
				sess.Rollback()
				return err
			}
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

// GetHighestAuthorize returns highest repository authorize level for given user and team.
func GetHighestAuthorize(orgId, uid, repoId, teamId int64) (AuthorizeType, error) {
	ts, err := GetUserTeams(orgId, uid)
	if err != nil {
		return 0, err
	}

	var auth AuthorizeType = 0
	for _, t := range ts {
		// Not current team and has given repository.
		if t.Id != teamId && strings.Contains(t.RepoIds, "$"+com.ToStr(repoId)+"|") {
			// Fast return.
			if t.Authorize == ORG_WRITABLE {
				return ORG_WRITABLE, nil
			}
			if t.Authorize > auth {
				auth = t.Authorize
			}
		}
	}

	return auth, nil
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
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	// Update access for team members if needed.
	if authChanged && !t.IsOwnerTeam() {
		if err = t.GetRepositories(); err != nil {
			return err
		} else if err = t.GetMembers(); err != nil {
			return err
		}

		// Get organization.
		org, err := GetUserById(t.OrgId)
		if err != nil {
			return err
		}

		// Update access.
		mode := AuthorizeToAccessType(t.Authorize)

		for _, repo := range t.Repos {
			for _, u := range t.Members {
				// ORG_WRITABLE is the highest authorize level for now.
				// Skip checking others if current team has this level.
				if t.Authorize < ORG_WRITABLE {
					auth, err := GetHighestAuthorize(t.OrgId, u.Id, repo.Id, t.Id)
					if err != nil {
						sess.Rollback()
						return err
					}
					if auth >= t.Authorize {
						continue // Other team has higher or same authorize level.
					}
				}

				access := &Access{
					UserName: u.LowerName,
					RepoName: path.Join(org.LowerName, repo.LowerName),
				}
				if err = addAccessWithAuthorize(sess, access, mode); err != nil {
					sess.Rollback()
					return err
				}
			}
		}
	}

	t.LowerName = strings.ToLower(t.Name)
	if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
		sess.Rollback()
		return err
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
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	// Delete all accesses.
	for _, repo := range t.Repos {
		for _, u := range t.Members {
			auth, err := GetHighestAuthorize(t.OrgId, u.Id, repo.Id, t.Id)
			if err != nil {
				sess.Rollback()
				return err
			}

			access := &Access{
				UserName: u.LowerName,
				RepoName: path.Join(org.LowerName, repo.LowerName),
			}
			if auth == 0 {
				if _, err = sess.Delete(access); err != nil {
					sess.Rollback()
					return fmt.Errorf("fail to delete access: %v", err)
				}
			} else if auth < t.Authorize {
				// Downgrade authorize level.
				if err = addAccessWithAuthorize(sess, access, AuthorizeToAccessType(auth)); err != nil {
					sess.Rollback()
					return err
				}
			}
		}
	}

	// Delete team-user.
	if _, err = sess.Where("org_id=?", org.Id).Where("team_id=?", t.Id).Delete(new(TeamUser)); err != nil {
		sess.Rollback()
		return err
	}

	// Delete team.
	if _, err = sess.Id(t.Id).Delete(new(Team)); err != nil {
		sess.Rollback()
		return err
	}
	// Update organization number of teams.
	if _, err = sess.Exec("UPDATE `user` SET num_teams = num_teams - 1 WHERE id = ?", t.OrgId); err != nil {
		sess.Rollback()
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

// IsTeamMember returns true if given user is a member of team.
func IsTeamMember(orgId, teamId, uid int64) bool {
	has, _ := x.Where("uid=?", uid).And("org_id=?", orgId).And("team_id=?", teamId).Get(new(TeamUser))
	return has
}

// GetTeamMembers returns all members in given team of organization.
func GetTeamMembers(orgId, teamId int64) ([]*User, error) {
	us := make([]*User, 0, 10)
	err := x.Sql("SELECT * FROM `user` JOIN `team_user` ON `team_user`.`team_id` = ? AND `team_user`.`uid` = `user`.`id`", teamId).Find(&us)
	return us, err
}

// GetUserTeams returns all teams that user belongs to in given organization.
func GetUserTeams(orgId, uid int64) ([]*Team, error) {
	tus := make([]*TeamUser, 0, 5)
	if err := x.Where("uid=?", uid).And("org_id=?", orgId).Find(&tus); err != nil {
		return nil, err
	}

	ts := make([]*Team, len(tus))
	for i, tu := range tus {
		t := new(Team)
		has, err := x.Id(tu.TeamId).Get(t)
		if err != nil {
			return nil, err
		} else if !has {
			return nil, ErrTeamNotExist
		}
		ts[i] = t
	}
	return ts, nil
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

	if _, err = sess.Insert(tu); err != nil {
		sess.Rollback()
		return err
	} else if _, err = sess.Id(t.Id).Update(t); err != nil {
		sess.Rollback()
		return err
	}

	// Give access to team repositories.
	mode := AuthorizeToAccessType(t.Authorize)
	for _, repo := range t.Repos {
		auth, err := GetHighestAuthorize(t.OrgId, u.Id, repo.Id, teamId)
		if err != nil {
			sess.Rollback()
			return err
		}

		access := &Access{
			UserName: u.LowerName,
			RepoName: path.Join(org.LowerName, repo.LowerName),
		}
		if auth < t.Authorize {
			if err = addAccessWithAuthorize(sess, access, mode); err != nil {
				sess.Rollback()
				return err
			}
		}
	}

	// We make sure it exists before.
	ou := new(OrgUser)
	_, err = sess.Where("uid=?", uid).And("org_id=?", orgId).Get(ou)
	if err != nil {
		sess.Rollback()
		return err
	}
	ou.NumTeams++
	if t.IsOwnerTeam() {
		ou.IsOwner = true
	}
	if _, err = sess.Id(ou.Id).AllCols().Update(ou); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

func removeTeamMemberWithSess(orgId, teamId, uid int64, sess *xorm.Session) error {
	if !IsTeamMember(orgId, teamId, uid) {
		return nil
	}

	// Get team and its repositories.
	t, err := GetTeamById(teamId)
	if err != nil {
		return err
	}

	// Check if the user to delete is the last member in owner team.
	if t.IsOwnerTeam() && t.NumMembers == 1 {
		return ErrLastOrgOwner
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

	tu := &TeamUser{
		Uid:    uid,
		OrgId:  orgId,
		TeamId: teamId,
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
		auth, err := GetHighestAuthorize(t.OrgId, u.Id, repo.Id, teamId)
		if err != nil {
			sess.Rollback()
			return err
		}

		access := &Access{
			UserName: u.LowerName,
			RepoName: path.Join(org.LowerName, repo.LowerName),
		}
		// Delete access if this is the last team user belongs to.
		if auth == 0 {
			if _, err = sess.Delete(access); err != nil {
				sess.Rollback()
				return fmt.Errorf("fail to delete access: %v", err)
			} else if err = WatchRepo(u.Id, repo.Id, false); err != nil {
				sess.Rollback()
				return err
			}
		} else if auth < t.Authorize {
			// Downgrade authorize level.
			if err = addAccessWithAuthorize(sess, access, AuthorizeToAccessType(auth)); err != nil {
				sess.Rollback()
				return err
			}
		}
	}

	// This must exist.
	ou := new(OrgUser)
	_, err = sess.Where("uid=?", uid).And("org_id=?", org.Id).Get(ou)
	if err != nil {
		sess.Rollback()
		return err
	}
	ou.NumTeams--
	if t.IsOwnerTeam() {
		ou.IsOwner = false
	}
	if _, err = sess.Id(ou.Id).AllCols().Update(ou); err != nil {
		sess.Rollback()
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
	if err := removeTeamMemberWithSess(orgId, teamId, uid, sess); err != nil {
		return err
	}
	return sess.Commit()
}
