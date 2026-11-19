package database

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"xorm.io/builder"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/userx"
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

func (org *User) getTeam(e Engine, name string) (*Team, error) {
	return getTeamOfOrgByName(e, org.ID, name)
}

// GetTeam returns named team of organization.
func (org *User) GetTeam(name string) (*Team, error) {
	return org.getTeam(x, name)
}

func (org *User) getOwnerTeam(e Engine) (*Team, error) {
	return org.getTeam(e, ownerTeamName)
}

// GetOwnerTeam returns owner team of organization.
func (org *User) GetOwnerTeam() (*Team, error) {
	return org.getOwnerTeam(x)
}

func (org *User) getTeams(tx *gorm.DB) (err error) {
	org.Teams, err = getTeamsByOrgID(tx, org.ID)
	return err
}

// GetTeams returns all teams that belong to organization.
func (org *User) GetTeams() error {
	return org.getTeams(Handle.db)
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

func (org *User) removeOrgRepo(e Engine, repoID int64) error {
	return removeOrgRepo(e, org.ID, repoID)
}

// RemoveOrgRepo removes all team-repository relations of organization.
func (org *User) RemoveOrgRepo(repoID int64) error {
	return org.removeOrgRepo(x, repoID)
}

// CreateOrganization creates record of a new organization.
func CreateOrganization(org, owner *User) (err error) {
	if err = isUsernameAllowed(org.Name); err != nil {
		return err
	}

	if Handle.Users().IsUsernameUsed(context.TODO(), org.Name, 0) {
		return ErrUserAlreadyExist{
			args: errx.Args{
				"name": org.Name,
			},
		}
	}

	org.LowerName = strings.ToLower(org.Name)
	if org.Rands, err = userx.RandomSalt(); err != nil {
		return err
	}
	if org.Salt, err = userx.RandomSalt(); err != nil {
		return err
	}
	org.UseCustomAvatar = true
	org.MaxRepoCreation = -1
	org.NumTeams = 1
	org.NumMembers = 1
	// The GORM create hook that stamps the creation time does not run for XORM inserts.
	org.CreatedUnix = time.Now().Unix()
	org.UpdatedUnix = org.CreatedUnix

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(org); err != nil {
		return errors.Newf("insert organization: %v", err)
	}
	_ = userx.GenerateRandomAvatar(org.ID, org.Name, org.Email)

	// Add initial creator to organization and owner team.
	if _, err = sess.Insert(&OrgUser{
		UID:      owner.ID,
		OrgID:    org.ID,
		IsOwner:  true,
		NumTeams: 1,
	}); err != nil {
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
	if _, err = sess.Insert(t); err != nil {
		return errors.Newf("insert owner team: %v", err)
	}

	if _, err = sess.Insert(&TeamUser{
		UID:    owner.ID,
		OrgID:  org.ID,
		TeamID: t.ID,
	}); err != nil {
		return errors.Newf("insert team-user relation: %v", err)
	}

	if err = os.MkdirAll(repox.UserPath(org.Name), os.ModePerm); err != nil {
		return errors.Newf("create directory: %v", err)
	}

	return sess.Commit()
}

// GetOrgByName returns organization by given name.
func GetOrgByName(name string) (*User, error) {
	if name == "" {
		return nil, ErrOrgNotExist
	}
	u := &User{
		LowerName: strings.ToLower(name),
		Type:      UserTypeOrganization,
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

// deleteByColumn deletes the rows of given tables whose column has the ID. It
// takes table names rather than models, because XORM cannot map the models
// that only GORM queries. A non-positive ID is refused, because a column with 0
// means "none" in some tables, e.g., webhooks of organizations have repo_id 0.
func deleteByColumn(e Engine, column string, id int64, tables ...string) error {
	if id <= 0 {
		return errors.Newf("delete by %s: invalid ID %d", column, id)
	}
	for _, table := range tables {
		if _, err := e.Exec("DELETE FROM `"+table+"` WHERE "+column+" = ?", id); err != nil {
			return errors.Newf("delete %s by %s: %v", table, column, err)
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

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	// Deliveries for repositories that have been transferred out are reachable
	// only through the webhooks, so they go with them.
	if _, err = sess.Where("hook_id IN (SELECT id FROM webhook WHERE org_id = ?)", org.ID).Delete(new(HookTask)); err != nil {
		return errors.Newf("delete deliveries of webhooks: %v", err)
	}
	if err = deleteByColumn(sess, "org_id", org.ID, "team", "org_user", "team_user", "webhook"); err != nil {
		return err
	}
	return sess.Commit()
}

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
	has, _ := x.Where("is_owner = ?", true).And("uid = ?", userID).And("org_id = ?", orgID).Get(new(OrgUser))
	return has
}

// IsOrganizationMember returns true if given user is member of organization.
func IsOrganizationMember(orgID, uid int64) bool {
	has, _ := x.Where("uid=?", uid).And("org_id=?", orgID).Get(new(OrgUser))
	return has
}

// IsPublicMembership returns true if given user has made their membership public.
func IsPublicMembership(orgID, uid int64) bool {
	has, _ := x.Where("uid=?", uid).And("org_id=?", orgID).And("is_public=?", true).Get(new(OrgUser))
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

func getOwnedOrgsByUserID(sess *xorm.Session, userID int64) ([]*User, error) {
	orgs := make([]*User, 0, 10)
	return orgs, sess.Where("`org_user`.uid=?", userID).And("`org_user`.is_owner=?", true).
		Join("INNER", "`org_user`", "`org_user`.org_id=`user`.id").Find(&orgs)
}

// GetOwnedOrgsByUserIDDesc returns a list of organizations that are owned by
// given user ID, ordered descending by the given condition.
func GetOwnedOrgsByUserIDDesc(userID int64, desc string) ([]*User, error) {
	sess := x.NewSession()
	return getOwnedOrgsByUserID(sess.Desc(desc), userID)
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
	return getOrgUsersByOrgID(Handle.db, orgID, limit)
}

// ChangeOrgUserStatus changes public or private membership status.
func ChangeOrgUserStatus(orgID, uid int64, public bool) error {
	ou := new(OrgUser)
	has, err := x.Where("uid=?", uid).And("org_id=?", orgID).Get(ou)
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
func AddOrgUser(orgID, uid int64) error {
	if IsOrganizationMember(orgID, uid) {
		return nil
	}

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	ou := &OrgUser{
		UID:   uid,
		OrgID: orgID,
	}

	if _, err := sess.Insert(ou); err != nil {
		return err
	} else if _, err = sess.Exec("UPDATE `user` SET num_members = num_members + 1 WHERE id = ?", orgID); err != nil {
		return err
	}

	return sess.Commit()
}

// RemoveOrgUser removes user from given organization.
func RemoveOrgUser(orgID, userID int64) error {
	ou := new(OrgUser)

	has, err := x.Where("uid=?", userID).And("org_id=?", orgID).Get(ou)
	if err != nil {
		return errors.Newf("get org-user: %v", err)
	} else if !has {
		return nil
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

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}

	if _, err := sess.ID(ou.ID).Delete(ou); err != nil {
		return err
	} else if _, err = sess.Exec("UPDATE `user` SET num_members=num_members-1 WHERE id=?", orgID); err != nil {
		return err
	}

	// Delete all repository accesses and unwatch them.
	repoIDs := make([]int64, 0, len(repos))
	for i := range repos {
		repoIDs = append(repoIDs, repos[i].ID)
		if err = newReposStore(legacyTx(sess)).unwatch(context.TODO(), user.ID, repos[i].ID); err != nil {
			return err
		}
	}

	if len(repoIDs) > 0 {
		if err = legacyTx(sess).Where("user_id = ? AND repo_id IN ?", user.ID, repoIDs).Delete(new(Access)).Error; err != nil {
			return err
		}
	}

	// Remove the member from their teams.
	teams, err := getUserTeams(sess, org.ID, user.ID)
	if err != nil {
		return err
	}
	for _, t := range teams {
		if err = removeTeamMember(sess, org.ID, t.ID, user.ID); err != nil {
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

func (org *User) getUserTeams(e Engine, userID int64, cols ...string) ([]*Team, error) {
	teams := make([]*Team, 0, org.NumTeams)
	return teams, e.Where("team_user.org_id = ?", org.ID).
		And("team_user.uid = ?", userID).
		Join("INNER", "team_user", "team_user.team_id = team.id").
		Cols(cols...).Find(&teams)
}

// GetUserTeamIDs returns all team IDs of the organization that user is member of.
func (org *User) GetUserTeamIDs(userID int64) ([]int64, error) {
	teams, err := org.getUserTeams(x, userID, "team.id")
	if err != nil {
		return nil, errors.Newf("getUserTeams [%d]: %v", userID, err)
	}

	teamIDs := make([]int64, len(teams))
	for i := range teams {
		teamIDs[i] = teams[i].ID
	}
	return teamIDs, nil
}

// GetUserTeams returns all teams that belong to organization,
// and that the user has joined.
func (org *User) GetUserTeams(userID int64) ([]*Team, error) {
	return org.getUserTeams(x, userID)
}

// GetUserRepositories returns a range of repositories in organization which the user has access to,
// and total number of records based on given condition.
func (org *User) GetUserRepositories(userID int64, page, pageSize int) ([]*Repository, int64, error) {
	teamIDs, err := org.GetUserTeamIDs(userID)
	if err != nil {
		return nil, 0, errors.Newf("GetUserTeamIDs: %v", err)
	}
	if len(teamIDs) == 0 {
		// User has no team but "IN ()" is invalid SQL.
		teamIDs = []int64{-1} // There is no team with id=-1.
	}

	var teamRepoIDs []int64
	if err = x.Table("team_repo").In("team_id", teamIDs).Distinct("repo_id").Find(&teamRepoIDs); err != nil {
		return nil, 0, errors.Newf("get team repository IDs: %v", err)
	}
	if len(teamRepoIDs) == 0 {
		// Team has no repo but "IN ()" is invalid SQL.
		teamRepoIDs = []int64{-1} // There is no repo with id=-1.
	}

	if page <= 0 {
		page = 1
	}
	repos := make([]*Repository, 0, pageSize)
	if err = x.Where("owner_id = ?", org.ID).
		And(builder.Or(
			builder.And(builder.Expr("is_private = ?", false), builder.Expr("is_unlisted = ?", false)),
			builder.In("id", teamRepoIDs))).
		Desc("updated_unix").
		Limit(pageSize, (page-1)*pageSize).
		Find(&repos); err != nil {
		return nil, 0, errors.Newf("get user repositories: %v", err)
	}

	repoCount, err := x.Where("owner_id = ?", org.ID).
		And(builder.Or(
			builder.Expr("is_private = ?", false),
			builder.In("id", teamRepoIDs))).
		Count(new(Repository))
	if err != nil {
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
	err = x.Table("team_repo").In("team_id", teamIDs).Distinct("repo_id").Find(&teamRepoIDs)
	if err != nil {
		return nil, errors.Newf("get team repository ids: %v", err)
	}
	if len(teamRepoIDs) == 0 {
		// Team has no repo but "IN ()" is invalid SQL.
		teamRepoIDs = []int64{-1} // There is no repo with id=-1.
	}

	repos := make([]*Repository, 0, 10)
	if err = x.Where("owner_id = ?", org.ID).
		And("is_private = ?", false).
		Or(builder.In("id", teamRepoIDs)).
		And("is_mirror = ?", true). // Don't move up because it's an independent condition.
		Desc("updated_unix").
		Find(&repos); err != nil {
		return nil, errors.Newf("get user repositories: %v", err)
	}
	return repos, nil
}
