package database

import (
	"context"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/repoutil"
	"gogs.io/gogs/internal/userutil"
)

var ErrOrgNotExist = errors.New("Organization does not exist")

// IsOwnedBy returns true if given user is in the owner team.
func (org *User) IsOwnedBy(userID int64) bool {
	return IsOrganizationOwner(org.ID, userID)
}

// IsOrgMember returns true if given user is member of organization.
func (org *User) IsOrgMember(uid int64) bool {
	return org.IsOrganization() && IsOrganizationMember(org.ID, uid)
}

func (org *User) getTeam(tx *gorm.DB, name string) (*Team, error) {
	return getTeamOfOrgByName(tx, org.ID, name)
}

// GetTeamOfOrgByName returns named team of organization.
func (org *User) GetTeam(name string) (*Team, error) {
	return org.getTeam(db, name)
}

func (org *User) getOwnerTeam(tx *gorm.DB) (*Team, error) {
	return org.getTeam(tx, ownerTeamName)
}

// GetOwnerTeam returns owner team of organization.
func (org *User) GetOwnerTeam() (*Team, error) {
	return org.getOwnerTeam(db)
}

func (org *User) getTeams(tx *gorm.DB) (err error) {
	org.Teams, err = getTeamsByOrgID(tx, org.ID)
	return err
}

// GetTeams returns all teams that belong to organization.
func (org *User) GetTeams() error {
	return org.getTeams(db)
}

// TeamsHaveAccessToRepo returns all teams that have given access level to the repository.
func (org *User) TeamsHaveAccessToRepo(repoID int64, mode AccessMode) ([]*Team, error) {
	return GetTeamsHaveAccessToRepo(org.ID, repoID, mode)
}

// GetMembers returns all members of organization.
func (org *User) GetMembers(limit int) error {
	ous, err := GetOrgUsersByOrgID(org.ID, limit)
	if err != nil {
		return err
	}

	org.Members = make([]*User, len(ous))
	for i, ou := range ous {
		org.Members[i], err = Handle.Users().GetByID(context.TODO(), ou.UID)
		if err != nil {
			return err
		}
	}
	return nil
}

// AddMember adds new member to organization.
func (org *User) AddMember(uid int64) error {
	return AddOrgUser(org.ID, uid)
}

// RemoveMember removes member from organization.
func (org *User) RemoveMember(uid int64) error {
	return RemoveOrgUser(org.ID, uid)
}

func (org *User) removeOrgRepo(tx *gorm.DB, repoID int64) error {
	return removeOrgRepo(tx, org.ID, repoID)
}

// RemoveOrgRepo removes all team-repository relations of organization.
func (org *User) RemoveOrgRepo(repoID int64) error {
	return org.removeOrgRepo(db, repoID)
}

// CreateOrganization creates record of a new organization.
func CreateOrganization(org, owner *User) (err error) {
	if err = isUsernameAllowed(org.Name); err != nil {
		return err
	}

	if Handle.Users().IsUsernameUsed(context.TODO(), org.Name, 0) {
		return ErrUserAlreadyExist{
			args: errutil.Args{
				"name": org.Name,
			},
		}
	}

	org.LowerName = strings.ToLower(org.Name)
	if org.Rands, err = userutil.RandomSalt(); err != nil {
		return err
	}
	if org.Salt, err = userutil.RandomSalt(); err != nil {
		return err
	}
	org.UseCustomAvatar = true
	org.MaxRepoCreation = -1
	org.NumTeams = 1
	org.NumMembers = 1

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(org).Error; err != nil {
			return errors.Newf("insert organization: %v", err)
		}
		_ = userutil.GenerateRandomAvatar(org.ID, org.Name, org.Email)

		// Add initial creator to organization and owner team.
		if err := tx.Create(&OrgUser{
			UID:      owner.ID,
			OrgID:    org.ID,
			IsOwner:  true,
			NumTeams: 1,
		}).Error; err != nil {
			return errors.Newf("insert org-user relation: %v", err)
		}

		// Create default owner team.
		t := &Team{
			OrgID:      org.ID,
			LowerName:  strings.ToLower(ownerTeamName),
			Name:       ownerTeamName,
			Authorize:  AccessModeOwner,
			NumMembers: 1,
		}
		if err := tx.Create(t).Error; err != nil {
			return errors.Newf("insert owner team: %v", err)
		}

		if err := tx.Create(&TeamUser{
			UID:    owner.ID,
			OrgID:  org.ID,
			TeamID: t.ID,
		}).Error; err != nil {
			return errors.Newf("insert team-user relation: %v", err)
		}

		if err := os.MkdirAll(repoutil.UserPath(org.Name), os.ModePerm); err != nil {
			return errors.Newf("create directory: %v", err)
		}

		return nil
	})
}

// GetOrgByName returns organization by given name.
func GetOrgByName(name string) (*User, error) {
	if name == "" {
		return nil, ErrOrgNotExist
	}
	u := &User{}
	err := db.Where("lower_name = ? AND type = ?", strings.ToLower(name), UserTypeOrganization).First(u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrgNotExist
		}
		return nil, err
	}
	return u, nil
}

// CountOrganizations returns number of organizations.
func CountOrganizations() int64 {
	var count int64
	db.Model(new(User)).Where("type = ?", UserTypeOrganization).Count(&count)
	return count
}

// Organizations returns number of organizations in given page.
func Organizations(page, pageSize int) ([]*User, error) {
	orgs := make([]*User, 0, pageSize)
	return orgs, db.Where("type = ?", UserTypeOrganization).
		Offset((page - 1) * pageSize).Limit(pageSize).
		Order("id ASC").Find(&orgs).Error
}

// deleteBeans deletes all given beans, beans should contain delete conditions.
func deleteBeans(tx *gorm.DB, beans ...any) (err error) {
	for i := range beans {
		if err = tx.Delete(beans[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// DeleteOrganization completely and permanently deletes everything of organization.
func DeleteOrganization(org *User) error {
	err := Handle.Users().DeleteByID(context.TODO(), org.ID, false)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		return deleteBeans(tx,
			&Team{OrgID: org.ID},
			&OrgUser{OrgID: org.ID},
			&TeamUser{OrgID: org.ID},
		)
	})
}

// ________                ____ ___
// \_____  \_______  ____ |    |   \______ ___________
//  /   |   \_  __ \/ ___\|    |   /  ___// __ \_  __ \
// /    |    \  | \/ /_/  >    |  /\___ \\  ___/|  | \/
// \_______  /__|  \___  /|______//____  >\___  >__|
//         \/     /_____/              \/     \/

// OrgUser represents relations of organizations and their members.
type OrgUser struct {
	ID       int64 `gorm:"primaryKey"`
	UID      int64 `xorm:"uid INDEX UNIQUE(s)" gorm:"column:uid;uniqueIndex:org_user_user_org_unique;index;not null"`
	OrgID    int64 `xorm:"INDEX UNIQUE(s)" gorm:"uniqueIndex:org_user_user_org_unique;index;not null"`
	IsPublic bool  `gorm:"not null;default:FALSE"`
	IsOwner  bool  `gorm:"not null;default:FALSE"`
	NumTeams int   `gorm:"not null;default:0"`
}

// IsOrganizationOwner returns true if given user is in the owner team.
func IsOrganizationOwner(orgID, userID int64) bool {
	var count int64
	db.Model(new(OrgUser)).Where("is_owner = ? AND uid = ? AND org_id = ?", true, userID, orgID).Count(&count)
	return count > 0
}

// IsOrganizationMember returns true if given user is member of organization.
func IsOrganizationMember(orgID, uid int64) bool {
	var count int64
	db.Model(new(OrgUser)).Where("uid = ? AND org_id = ?", uid, orgID).Count(&count)
	return count > 0
}

// IsPublicMembership returns true if given user public his/her membership.
func IsPublicMembership(orgID, uid int64) bool {
	var count int64
	db.Model(new(OrgUser)).Where("uid = ? AND org_id = ? AND is_public = ?", uid, orgID, true).Count(&count)
	return count > 0
}

func getOrgsByUserID(tx *gorm.DB, userID int64, showAll bool) ([]*User, error) {
	orgs := make([]*User, 0, 10)
	query := tx.Table("`user`").
		Joins("INNER JOIN `org_user` ON `org_user`.org_id = `user`.id").
		Where("`org_user`.uid = ?", userID)
	if !showAll {
		query = query.Where("`org_user`.is_public = ?", true)
	}
	return orgs, query.Find(&orgs).Error
}

// GetOrgsByUserID returns a list of organizations that the given user ID
// has joined.
func GetOrgsByUserID(userID int64, showAll bool) ([]*User, error) {
	return getOrgsByUserID(db, userID, showAll)
}

func getOwnedOrgsByUserID(tx *gorm.DB, userID int64) ([]*User, error) {
	orgs := make([]*User, 0, 10)
	return orgs, tx.Table("`user`").
		Joins("INNER JOIN `org_user` ON `org_user`.org_id = `user`.id").
		Where("`org_user`.uid = ? AND `org_user`.is_owner = ?", userID, true).
		Find(&orgs).Error
}

// GetOwnedOrgsByUserID returns a list of organizations are owned by given user ID.
func GetOwnedOrgsByUserID(userID int64) ([]*User, error) {
	return getOwnedOrgsByUserID(db, userID)
}

// GetOwnedOrganizationsByUserIDDesc returns a list of organizations are owned by
// given user ID, ordered descending by the given condition.
func GetOwnedOrgsByUserIDDesc(userID int64, desc string) ([]*User, error) {
	orgs := make([]*User, 0, 10)
	return orgs, db.Table("`user`").
		Joins("INNER JOIN `org_user` ON `org_user`.org_id = `user`.id").
		Where("`org_user`.uid = ? AND `org_user`.is_owner = ?", userID, true).
		Order(desc + " DESC").
		Find(&orgs).Error
}

func getOrgUsersByOrgID(tx *gorm.DB, orgID int64, limit int) ([]*OrgUser, error) {
	orgUsers := make([]*OrgUser, 0, 10)

	query := tx.Where("org_id = ?", orgID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	return orgUsers, query.Find(&orgUsers).Error
}

// GetOrgUsersByOrgID returns all organization-user relations by organization ID.
func GetOrgUsersByOrgID(orgID int64, limit int) ([]*OrgUser, error) {
	return getOrgUsersByOrgID(db, orgID, limit)
}

// ChangeOrgUserStatus changes public or private membership status.
func ChangeOrgUserStatus(orgID, uid int64, public bool) error {
	ou := new(OrgUser)
	err := db.Where("uid = ? AND org_id = ?", uid, orgID).First(ou).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	ou.IsPublic = public
	return db.Model(ou).Where("id = ?", ou.ID).Updates(ou).Error
}

// AddOrgUser adds new user to given organization.
func AddOrgUser(orgID, uid int64) error {
	if IsOrganizationMember(orgID, uid) {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		ou := &OrgUser{
			UID:   uid,
			OrgID: orgID,
		}

		if err := tx.Create(ou).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE `user` SET num_members = num_members + 1 WHERE id = ?", orgID).Error; err != nil {
			return err
		}

		return nil
	})
}

// RemoveOrgUser removes user from given organization.
func RemoveOrgUser(orgID, userID int64) error {
	ou := new(OrgUser)

	err := db.Where("uid = ? AND org_id = ?", userID, orgID).First(ou).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return errors.Newf("get org-user: %v", err)
	}

	user, err := Handle.Users().GetByID(context.TODO(), userID)
	if err != nil {
		return errors.Newf("GetUserByID [%d]: %v", userID, err)
	}
	org, err := Handle.Users().GetByID(context.TODO(), orgID)
	if err != nil {
		return errors.Newf("GetUserByID [%d]: %v", orgID, err)
	}

	// FIXME: only need to get IDs here, not all fields of repository.
	repos, _, err := org.GetUserRepositories(user.ID, 1, org.NumRepos)
	if err != nil {
		return errors.Newf("GetUserRepositories [%d]: %v", user.ID, err)
	}

	// Check if the user to delete is the last member in owner team.
	if IsOrganizationOwner(orgID, userID) {
		t, err := org.GetOwnerTeam()
		if err != nil {
			return err
		}
		if t.NumMembers == 1 {
			return ErrLastOrgOwner{UID: userID}
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", ou.ID).Delete(ou).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE `user` SET num_members = num_members - 1 WHERE id = ?", orgID).Error; err != nil {
			return err
		}

		// Delete all repository accesses and unwatch them.
		repoIDs := make([]int64, 0, len(repos))
		for i := range repos {
			repoIDs = append(repoIDs, repos[i].ID)
			if err = watchRepo(tx, user.ID, repos[i].ID, false); err != nil {
				return err
			}
		}

		if len(repoIDs) > 0 {
			if err := tx.Where("user_id = ? AND repo_id IN ?", user.ID, repoIDs).Delete(new(Access)).Error; err != nil {
				return err
			}
		}

		// Delete member in his/her teams.
		teams, err := getUserTeams(tx, org.ID, user.ID)
		if err != nil {
			return err
		}
		for _, t := range teams {
			if err = removeTeamMember(tx, org.ID, t.ID, user.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

func removeOrgRepo(tx *gorm.DB, orgID, repoID int64) error {
	return tx.Where("org_id = ? AND repo_id = ?", orgID, repoID).Delete(&TeamRepo{}).Error
}

// RemoveOrgRepo removes all team-repository relations of given organization.
func RemoveOrgRepo(orgID, repoID int64) error {
	return removeOrgRepo(db, orgID, repoID)
}

func (org *User) getUserTeams(tx *gorm.DB, userID int64, cols ...string) ([]*Team, error) {
	teams := make([]*Team, 0, org.NumTeams)
	query := tx.Table("team").
		Joins("INNER JOIN team_user ON team_user.team_id = team.id").
		Where("team_user.org_id = ? AND team_user.uid = ?", org.ID, userID)

	if len(cols) > 0 {
		query = query.Select(cols)
	}

	return teams, query.Find(&teams).Error
}

// GetUserTeamIDs returns of all team IDs of the organization that user is member of.
func (org *User) GetUserTeamIDs(userID int64) ([]int64, error) {
	teams, err := org.getUserTeams(db, userID, "team.id")
	if err != nil {
		return nil, errors.Newf("getUserTeams [%d]: %v", userID, err)
	}

	teamIDs := make([]int64, len(teams))
	for i := range teams {
		teamIDs[i] = teams[i].ID
	}
	return teamIDs, nil
}

// GetTeams returns all teams that belong to organization,
// and that the user has joined.
func (org *User) GetUserTeams(userID int64) ([]*Team, error) {
	return org.getUserTeams(db, userID)
}

// GetUserRepositories returns a range of repositories in organization which the user has access to,
// and total number of records based on given condition.
func (org *User) GetUserRepositories(userID int64, page, pageSize int) ([]*Repository, int64, error) {
	teamIDs, err := org.GetUserTeamIDs(userID)
	if err != nil {
		return nil, 0, errors.Newf("GetUserTeamIDs: %v", err)
	}
	if len(teamIDs) == 0 {
		// user has no team but "IN ()" is invalid SQL
		teamIDs = []int64{-1} // there is no team with id=-1
	}

	var teamRepoIDs []int64
	if err = db.Table("team_repo").Where("team_id IN ?", teamIDs).Distinct("repo_id").Find(&teamRepoIDs).Error; err != nil {
		return nil, 0, errors.Newf("get team repository IDs: %v", err)
	}
	if len(teamRepoIDs) == 0 {
		// team has no repo but "IN ()" is invalid SQL
		teamRepoIDs = []int64{-1} // there is no repo with id=-1
	}

	if page <= 0 {
		page = 1
	}
	repos := make([]*Repository, 0, pageSize)
	if err = db.Where("owner_id = ?", org.ID).
		Where(db.Where("is_private = ? AND is_unlisted = ?", false, false).Or("id IN ?", teamRepoIDs)).
		Order("updated_unix DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&repos).Error; err != nil {
		return nil, 0, errors.Newf("get user repositories: %v", err)
	}

	var repoCount int64
	if err = db.Model(&Repository{}).Where("owner_id = ?", org.ID).
		Where(db.Where("is_private = ?", false).Or("id IN ?", teamRepoIDs)).
		Count(&repoCount).Error; err != nil {
		return nil, 0, errors.Newf("count user repositories: %v", err)
	}

	return repos, repoCount, nil
}

// GetUserMirrorRepositories returns mirror repositories of the organization which the user has access to.
func (org *User) GetUserMirrorRepositories(userID int64) ([]*Repository, error) {
	teamIDs, err := org.GetUserTeamIDs(userID)
	if err != nil {
		return nil, errors.Newf("GetUserTeamIDs: %v", err)
	}
	if len(teamIDs) == 0 {
		teamIDs = []int64{-1}
	}

	var teamRepoIDs []int64
	err = db.Table("team_repo").Where("team_id IN ?", teamIDs).Distinct("repo_id").Find(&teamRepoIDs).Error
	if err != nil {
		return nil, errors.Newf("get team repository ids: %v", err)
	}
	if len(teamRepoIDs) == 0 {
		// team has no repo but "IN ()" is invalid SQL
		teamRepoIDs = []int64{-1} // there is no repo with id=-1
	}

	repos := make([]*Repository, 0, 10)
	if err = db.Where("owner_id = ?", org.ID).
		Where("is_private = ?", false).
		Or("id IN ?", teamRepoIDs).
		Where("is_mirror = ?", true). // Don't move up because it's an independent condition
		Order("updated_unix DESC").
		Find(&repos).Error; err != nil {
		return nil, errors.Newf("get user repositories: %v", err)
	}
	return repos, nil
}
