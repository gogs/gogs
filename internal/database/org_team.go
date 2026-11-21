package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/errx"
)

const ownerTeamName = "Owners"

// Team represents an organization team.
type Team struct {
	ID          int64
	OrgID       int64 `xorm:"INDEX"`
	LowerName   string
	Name        string
	Description string
	Authorize   AccessMode
	Repos       []*Repository `xorm:"-" json:"-" gorm:"-"`
	Members     []*User       `xorm:"-" json:"-" gorm:"-"`
	NumRepos    int
	NumMembers  int
}

// AfterFind implements the GORM query hook.
func (t *Team) AfterFind(_ *gorm.DB) error {
	// LEGACY [1.0]: this is backward compatibility bug fix for https://github.com/gogs/gogs/issues/3671
	if t.NumRepos < 0 {
		t.NumRepos = 0
	}
	return nil
}

func (t *Team) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "num_repos":
		// LEGACY [1.0]: this is backward compatibility bug fix for https://github.com/gogs/gogs/issues/3671
		if t.NumRepos < 0 {
			t.NumRepos = 0
		}
	}
}

// IsOwnerTeam returns true if team is owner team.
func (t *Team) IsOwnerTeam() bool {
	return t.Name == ownerTeamName
}

// HasWriteAccess returns true if team has at least write level access mode.
func (t *Team) HasWriteAccess() bool {
	return t.Authorize >= AccessModeWrite
}

// IsMember returns true if given user is a member of team.
func (t *Team) IsMember(userID int64) bool {
	return IsTeamMember(t.OrgID, t.ID, userID)
}

func (t *Team) getRepositories(e Engine) (err error) {
	teamRepos := make([]*TeamRepo, 0, t.NumRepos)
	if err = x.Where("team_id=?", t.ID).Find(&teamRepos); err != nil {
		return errors.Newf("get team-repos: %v", err)
	}

	t.Repos = make([]*Repository, 0, len(teamRepos))
	for i := range teamRepos {
		repo, err := getRepositoryByID(legacyTx(e), teamRepos[i].RepoID)
		if err != nil {
			return errors.Newf("getRepositoryById(%d): %v", teamRepos[i].RepoID, err)
		}
		t.Repos = append(t.Repos, repo)
	}
	return nil
}

// GetRepositories returns all repositories in team of organization.
func (t *Team) GetRepositories() error {
	return t.getRepositories(x)
}

func (t *Team) getMembers(tx *gorm.DB) (err error) {
	t.Members, err = getTeamMembers(tx, t.ID)
	return err
}

// GetMembers returns all members in team of organization.
func (t *Team) GetMembers() (err error) {
	return t.getMembers(Handle.db)
}

// AddMember adds new membership of the team to the organization,
// the user will have membership to the organization automatically when needed.
func (t *Team) AddMember(uid int64) error {
	return AddTeamMember(t.OrgID, t.ID, uid)
}

// RemoveMember removes member from team of organization.
func (t *Team) RemoveMember(uid int64) error {
	return RemoveTeamMember(t.OrgID, t.ID, uid)
}

func (t *Team) hasRepository(tx *gorm.DB, repoID int64) bool {
	return hasTeamRepo(tx, t.OrgID, t.ID, repoID)
}

// HasRepository returns true if given repository belongs to team.
func (t *Team) HasRepository(repoID int64) bool {
	return t.hasRepository(Handle.db, repoID)
}

func (t *Team) addRepository(e Engine, repo *Repository) (err error) {
	if err = addTeamRepo(e, t.OrgID, t.ID, repo.ID); err != nil {
		return err
	}

	t.NumRepos++
	if _, err = e.ID(t.ID).AllCols().Update(t); err != nil {
		return errors.Newf("update team: %v", err)
	}

	if err = newPermissionsStore(legacyTx(e)).recalculateTeamAccesses(context.TODO(), repo, 0); err != nil {
		return errors.Newf("recalculateAccesses: %v", err)
	}

	if err = t.getMembers(legacyTx(e)); err != nil {
		return errors.Newf("getMembers: %v", err)
	}
	for _, u := range t.Members {
		if err = newReposStore(legacyTx(e)).watch(context.TODO(), u.ID, repo.ID); err != nil {
			return errors.Newf("watchRepo: %v", err)
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
	defer sess.Close()
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
	if _, err = e.ID(t.ID).AllCols().Update(t); err != nil {
		return err
	}

	// No need to recalculate when deleting a repository from organization.
	if recalculate {
		if err = newPermissionsStore(legacyTx(e)).recalculateTeamAccesses(context.TODO(), repo, t.ID); err != nil {
			return err
		}
	}

	if err = t.getMembers(legacyTx(e)); err != nil {
		return errors.Newf("get team members: %v", err)
	}

	// TODO: Delete me when this method is migrated to use GORM.
	userAccessMode := func(e Engine, userID int64, repo *Repository) (AccessMode, error) {
		mode := AccessModeNone
		// Everyone has read access to public repository.
		if !repo.IsPrivate {
			mode = AccessModeRead
		}

		if userID <= 0 {
			return mode, nil
		}

		if userID == repo.OwnerID {
			return AccessModeOwner, nil
		}

		access := &Access{
			UserID: userID,
			RepoID: repo.ID,
		}
		if has, err := e.Get(access); !has || err != nil {
			return mode, err
		}
		return access.Mode, nil
	}

	hasAccess := func(e Engine, userID int64, repo *Repository, testMode AccessMode) (bool, error) {
		mode, err := userAccessMode(e, userID, repo)
		return mode >= testMode, err
	}

	for _, member := range t.Members {
		has, err := hasAccess(e, member.ID, repo, AccessModeRead)
		if err != nil {
			return err
		} else if has {
			continue
		}

		if err = newReposStore(legacyTx(e)).unwatch(context.TODO(), member.ID, repo.ID); err != nil {
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

	repo, err := getRepositoryByID(Handle.db, repoID)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = t.removeRepository(sess, repo, true); err != nil {
		return err
	}

	return sess.Commit()
}

var reservedTeamNames = map[string]struct{}{
	"new": {},
}

// IsUsableTeamName returns an error if given name is a reserved name or pattern.
func IsUsableTeamName(name string) error {
	return isNameAllowed(reservedTeamNames, nil, name)
}

// NewTeam creates a record of new team.
// It's caller's responsibility to assign organization ID.
func NewTeam(t *Team) error {
	if t.Name == "" {
		return errors.New("empty team name")
	} else if t.OrgID == 0 {
		return errors.New("OrgID is not assigned")
	}

	if err := IsUsableTeamName(t.Name); err != nil {
		return err
	}

	has, err := x.Id(t.OrgID).Get(new(User))
	if err != nil {
		return err
	} else if !has {
		return ErrOrgNotExist
	}

	t.LowerName = strings.ToLower(t.Name)
	existingTeam := Team{}
	has, err = x.Where("org_id=?", t.OrgID).And("lower_name=?", t.LowerName).Get(&existingTeam)
	if err != nil {
		return err
	} else if has {
		return ErrTeamAlreadyExist{existingTeam.ID, t.OrgID, t.LowerName}
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(t); err != nil {
		return err
	}

	// Update organization number of teams.
	if _, err = sess.Exec("UPDATE `user` SET num_teams=num_teams+1 WHERE id = ?", t.OrgID); err != nil {
		return err
	}
	return sess.Commit()
}

var _ errx.NotFound = (*ErrTeamNotExist)(nil)

type ErrTeamNotExist struct {
	args map[string]any
}

func IsErrTeamNotExist(err error) bool {
	_, ok := err.(ErrTeamNotExist)
	return ok
}

func (err ErrTeamNotExist) Error() string {
	return fmt.Sprintf("team does not exist: %v", err.args)
}

func (ErrTeamNotExist) NotFound() bool {
	return true
}

func getTeamOfOrgByName(e Engine, orgID int64, name string) (*Team, error) {
	t := &Team{
		OrgID:     orgID,
		LowerName: strings.ToLower(name),
	}
	has, err := e.Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrTeamNotExist{args: map[string]any{"orgID": orgID, "name": name}}
	}
	return t, nil
}

// GetTeamOfOrgByName returns team by given team name and organization.
func GetTeamOfOrgByName(orgID int64, name string) (*Team, error) {
	return getTeamOfOrgByName(x, orgID, name)
}

func getTeamByID(e Engine, teamID int64) (*Team, error) {
	t := new(Team)
	has, err := e.ID(teamID).Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrTeamNotExist{args: map[string]any{"teamID": teamID}}
	}
	return t, nil
}

// GetTeamByID returns team by given ID.
func GetTeamByID(teamID int64) (*Team, error) {
	return getTeamByID(x, teamID)
}

func getTeamsByOrgID(tx *gorm.DB, orgID int64) ([]*Team, error) {
	teams := make([]*Team, 0, 3)
	return teams, tx.Where("org_id = ?", orgID).Find(&teams).Error
}

// UpdateTeam updates information of team.
func UpdateTeam(t *Team, authChanged bool) (err error) {
	if t.Name == "" {
		return errors.New("empty team name")
	}

	if len(t.Description) > 255 {
		t.Description = t.Description[:255]
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	t.LowerName = strings.ToLower(t.Name)
	existingTeam := new(Team)
	has, err := x.Where("org_id=?", t.OrgID).And("lower_name=?", t.LowerName).And("id!=?", t.ID).Get(existingTeam)
	if err != nil {
		return err
	} else if has {
		return ErrTeamAlreadyExist{existingTeam.ID, t.OrgID, t.LowerName}
	}

	if _, err = sess.ID(t.ID).AllCols().Update(t); err != nil {
		return errors.Newf("update: %v", err)
	}

	// Update access for team members if needed.
	if authChanged {
		if err = t.getRepositories(sess); err != nil {
			return errors.Newf("getRepositories:%v", err)
		}

		for _, repo := range t.Repos {
			if err = newPermissionsStore(legacyTx(sess)).recalculateTeamAccesses(context.TODO(), repo, 0); err != nil {
				return errors.Newf("recalculateTeamAccesses: %v", err)
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

	org, err := Handle.Users().GetByID(context.TODO(), t.OrgID)
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
		if err = newPermissionsStore(legacyTx(sess)).recalculateTeamAccesses(context.TODO(), repo, t.ID); err != nil {
			return err
		}
	}

	if _, err = sess.Where("org_id=?", org.ID).Where("team_id=?", t.ID).Delete(new(TeamUser)); err != nil {
		return err
	}

	if _, err = sess.ID(t.ID).Delete(new(Team)); err != nil {
		return err
	}
	// Update organization number of teams.
	if _, err = sess.Exec("UPDATE `user` SET num_teams=num_teams-1 WHERE id=?", t.OrgID); err != nil {
		return err
	}

	return sess.Commit()
}

// TeamUser represents a team-user relation.
type TeamUser struct {
	ID     int64
	OrgID  int64 `xorm:"INDEX"`
	TeamID int64 `xorm:"UNIQUE(s)"`
	UID    int64 `xorm:"UNIQUE(s)"`
}

func isTeamMember(e Engine, orgID, teamID, uid int64) bool {
	has, _ := e.Where("org_id=?", orgID).And("team_id=?", teamID).And("uid=?", uid).Get(new(TeamUser))
	return has
}

// IsTeamMember returns true if given user is a member of team.
func IsTeamMember(orgID, teamID, uid int64) bool {
	return isTeamMember(x, orgID, teamID, uid)
}

func getTeamMembers(tx *gorm.DB, teamID int64) (_ []*User, err error) {
	teamUsers := make([]*TeamUser, 0, 10)
	if err = tx.Where("team_id = ?", teamID).Find(&teamUsers).Error; err != nil {
		return nil, errors.Newf("get team-users: %v", err)
	}
	members := make([]*User, 0, len(teamUsers))
	for i := range teamUsers {
		// NOTE: A missing user yields an empty member rather than an error.
		member := new(User)
		if err = tx.Where("id = ?", teamUsers[i].UID).Limit(1).Find(member).Error; err != nil {
			return nil, errors.Newf("get user '%d': %v", teamUsers[i].UID, err)
		}
		members = append(members, member)
	}
	return members, nil
}

func getUserTeams(e Engine, orgID, userID int64) ([]*Team, error) {
	teamUsers := make([]*TeamUser, 0, 5)
	if err := e.Where("uid = ?", userID).And("org_id = ?", orgID).Find(&teamUsers); err != nil {
		return nil, err
	}

	teamIDs := make([]int64, len(teamUsers)+1)
	for i := range teamUsers {
		teamIDs[i] = teamUsers[i].TeamID
	}
	teamIDs[len(teamUsers)] = -1

	teams := make([]*Team, 0, len(teamIDs))
	return teams, e.Where("org_id = ?", orgID).In("id", teamIDs).Find(&teams)
}

// GetUserTeams returns all teams that user belongs to in given organization.
func GetUserTeams(orgID, userID int64) ([]*Team, error) {
	return getUserTeams(x, orgID, userID)
}

// AddTeamMember adds new membership of given team to given organization,
// the user will have membership to given organization automatically when needed.
func AddTeamMember(orgID, teamID, userID int64) error {
	if IsTeamMember(orgID, teamID, userID) {
		return nil
	}

	if err := AddOrgUser(orgID, userID); err != nil {
		return err
	}

	// Get team and its repositories.
	t, err := GetTeamByID(teamID)
	if err != nil {
		return err
	}
	t.NumMembers++

	if err = t.GetRepositories(); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	tu := &TeamUser{
		UID:    userID,
		OrgID:  orgID,
		TeamID: teamID,
	}
	if _, err = sess.Insert(tu); err != nil {
		return err
	} else if _, err = sess.ID(t.ID).Update(t); err != nil {
		return err
	}

	// Give access to team repositories.
	for _, repo := range t.Repos {
		if err = newPermissionsStore(legacyTx(sess)).recalculateTeamAccesses(context.TODO(), repo, 0); err != nil {
			return err
		}
	}

	// AddOrgUser above makes sure it exists.
	ou := new(OrgUser)
	if _, err = sess.Where("uid = ?", userID).And("org_id = ?", orgID).Get(ou); err != nil {
		return err
	}
	ou.NumTeams++
	if t.IsOwnerTeam() {
		ou.IsOwner = true
	}
	if _, err = sess.ID(ou.ID).AllCols().Update(ou); err != nil {
		return err
	}

	return sess.Commit()
}

func removeTeamMember(e Engine, orgID, teamID, uid int64) error {
	if !isTeamMember(e, orgID, teamID, uid) {
		return nil
	}

	// Get team and its repositories.
	t, err := getTeamByID(e, teamID)
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

	org, err := newUsersStore(legacyTx(e)).GetByID(context.TODO(), orgID)
	if err != nil {
		return err
	}

	tu := &TeamUser{
		UID:    uid,
		OrgID:  orgID,
		TeamID: teamID,
	}
	if _, err := e.Delete(tu); err != nil {
		return err
	} else if _, err = e.ID(t.ID).AllCols().Update(t); err != nil {
		return err
	}

	// Delete access to team repositories.
	for _, repo := range t.Repos {
		if err = newPermissionsStore(legacyTx(e)).recalculateTeamAccesses(context.TODO(), repo, 0); err != nil {
			return err
		}
	}

	// This must exist.
	ou := new(OrgUser)
	_, err = e.Where("uid = ?", uid).And("org_id = ?", org.ID).Get(ou)
	if err != nil {
		return err
	}
	ou.NumTeams--
	if t.IsOwnerTeam() {
		ou.IsOwner = false
	}
	if _, err = e.ID(ou.ID).AllCols().Update(ou); err != nil {
		return err
	}
	return nil
}

// RemoveTeamMember removes member from given team of given organization.
func RemoveTeamMember(orgID, teamID, uid int64) error {
	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	if err := removeTeamMember(sess, orgID, teamID, uid); err != nil {
		return err
	}
	return sess.Commit()
}

// TeamRepo represents a team-repository relation.
type TeamRepo struct {
	ID     int64
	OrgID  int64 `xorm:"INDEX"`
	TeamID int64 `xorm:"UNIQUE(s)"`
	RepoID int64 `xorm:"UNIQUE(s)"`
}

func hasTeamRepo(tx *gorm.DB, orgID, teamID, repoID int64) bool {
	var count int64
	_ = tx.Model(new(TeamRepo)).Where("org_id = ? AND team_id = ? AND repo_id = ?", orgID, teamID, repoID).Count(&count).Error
	return count > 0
}

func addTeamRepo(e Engine, orgID, teamID, repoID int64) error {
	_, err := e.InsertOne(&TeamRepo{
		OrgID:  orgID,
		TeamID: teamID,
		RepoID: repoID,
	})
	return err
}

func removeTeamRepo(e Engine, teamID, repoID int64) error {
	_, err := e.Delete(&TeamRepo{
		TeamID: teamID,
		RepoID: repoID,
	})
	return err
}

// GetTeamsHaveAccessToRepo returns all teams in an organization that have given access level to the repository.
func GetTeamsHaveAccessToRepo(orgID, repoID int64, mode AccessMode) ([]*Team, error) {
	teams := make([]*Team, 0, 5)
	return teams, x.Where("team.authorize >= ?", mode).
		Join("INNER", "team_repo", "team_repo.team_id = team.id").
		And("team_repo.org_id = ?", orgID).
		And("team_repo.repo_id = ?", repoID).
		Find(&teams)
}
