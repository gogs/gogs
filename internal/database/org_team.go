package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errutil"
)

const ownerTeamName = "Owners"

// Team represents a organization team.
type Team struct {
	ID          int64
	OrgID       int64 `gorm:"index"`
	LowerName   string
	Name        string
	Description string
	Authorize   AccessMode
	Repos       []*Repository `xorm:"-" json:"-" gorm:"-"`
	Members     []*User       `xorm:"-" json:"-" gorm:"-"`
	NumRepos    int
	NumMembers  int
}

func (t *Team) AfterFind(tx *gorm.DB) error {
	// LEGACY [1.0]: this is backward compatibility bug fix for https://gogs.io/gogs/issues/3671
	if t.NumRepos < 0 {
		t.NumRepos = 0
	}
	return nil
}

// IsOwnerTeam returns true if team is owner team.
func (t *Team) IsOwnerTeam() bool {
	return t.Name == ownerTeamName
}

// HasWriteAccess returns true if team has at least write level access mode.
func (t *Team) HasWriteAccess() bool {
	return t.Authorize >= AccessModeWrite
}

// IsTeamMember returns true if given user is a member of team.
func (t *Team) IsMember(userID int64) bool {
	return IsTeamMember(t.OrgID, t.ID, userID)
}

func (t *Team) getRepositories(tx *gorm.DB) (err error) {
	teamRepos := make([]*TeamRepo, 0, t.NumRepos)
	if err = tx.Where("team_id = ?", t.ID).Find(&teamRepos).Error; err != nil {
		return errors.Newf("get team-repos: %v", err)
	}

	t.Repos = make([]*Repository, 0, len(teamRepos))
	for i := range teamRepos {
		repo, err := getRepositoryByID(tx, teamRepos[i].RepoID)
		if err != nil {
			return errors.Newf("getRepositoryById(%d): %v", teamRepos[i].RepoID, err)
		}
		t.Repos = append(t.Repos, repo)
	}
	return nil
}

// GetRepositories returns all repositories in team of organization.
func (t *Team) GetRepositories() error {
	return t.getRepositories(db)
}

func (t *Team) getMembers(tx *gorm.DB) (err error) {
	t.Members, err = getTeamMembers(tx, t.ID)
	return err
}

// GetMembers returns all members in team of organization.
func (t *Team) GetMembers() (err error) {
	return t.getMembers(db)
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

// HasRepository returns true if given repository belong to team.
func (t *Team) HasRepository(repoID int64) bool {
	return t.hasRepository(db, repoID)
}

func (t *Team) addRepository(tx *gorm.DB, repo *Repository) (err error) {
	if err = addTeamRepo(tx, t.OrgID, t.ID, repo.ID); err != nil {
		return err
	}

	t.NumRepos++
	if err = tx.Model(&Team{}).Where("id = ?", t.ID).Updates(t).Error; err != nil {
		return errors.Newf("update team: %v", err)
	}

	if err = repo.recalculateTeamAccesses(tx, 0); err != nil {
		return errors.Newf("recalculateAccesses: %v", err)
	}

	if err = t.getMembers(tx); err != nil {
		return errors.Newf("getMembers: %v", err)
	}
	for _, u := range t.Members {
		if err = watchRepo(tx, u.ID, repo.ID, true); err != nil {
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

	return db.Transaction(func(tx *gorm.DB) error {
		return t.addRepository(tx, repo)
	})
}

func (t *Team) removeRepository(tx *gorm.DB, repo *Repository, recalculate bool) (err error) {
	if err = removeTeamRepo(tx, t.ID, repo.ID); err != nil {
		return err
	}

	t.NumRepos--
	if err = tx.Model(&Team{}).Where("id = ?", t.ID).Updates(t).Error; err != nil {
		return err
	}

	// Don't need to recalculate when delete a repository from organization.
	if recalculate {
		if err = repo.recalculateTeamAccesses(tx, t.ID); err != nil {
			return err
		}
	}

	if err = t.getMembers(tx); err != nil {
		return errors.Newf("get team members: %v", err)
	}

	// TODO: Delete me when this method is migrated to use GORM.
	userAccessMode := func(tx *gorm.DB, userID int64, repo *Repository) (AccessMode, error) {
		mode := AccessModeNone
		// Everyone has read access to public repository
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
		err := tx.Where("user_id = ? AND repo_id = ?", userID, repo.ID).First(access).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mode, nil
		} else if err != nil {
			return mode, err
		}
		return access.Mode, nil
	}

	hasAccess := func(tx *gorm.DB, userID int64, repo *Repository, testMode AccessMode) (bool, error) {
		mode, err := userAccessMode(tx, userID, repo)
		return mode >= testMode, err
	}

	for _, member := range t.Members {
		has, err := hasAccess(tx, member.ID, repo, AccessModeRead)
		if err != nil {
			return err
		} else if has {
			continue
		}

		if err = watchRepo(tx, member.ID, repo.ID, false); err != nil {
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

	return db.Transaction(func(tx *gorm.DB) error {
		return t.removeRepository(tx, repo, true)
	})
}

var reservedTeamNames = map[string]struct{}{
	"new": {},
}

// IsUsableTeamName return an error if given name is a reserved name or pattern.
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

	err := db.Where("id = ?", t.OrgID).First(new(User)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrOrgNotExist
	} else if err != nil {
		return err
	}

	t.LowerName = strings.ToLower(t.Name)
	existingTeam := Team{}
	err = db.Where("org_id = ? AND lower_name = ?", t.OrgID, t.LowerName).First(&existingTeam).Error
	if err == nil {
		return ErrTeamAlreadyExist{existingTeam.ID, t.OrgID, t.LowerName}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}

		// Update organization number of teams.
		return tx.Exec("UPDATE `user` SET num_teams=num_teams+1 WHERE id = ?", t.OrgID).Error
	})
}

var _ errutil.NotFound = (*ErrTeamNotExist)(nil)

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

func getTeamOfOrgByName(tx *gorm.DB, orgID int64, name string) (*Team, error) {
	t := new(Team)
	err := tx.Where("org_id = ? AND lower_name = ?", orgID, strings.ToLower(name)).First(t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTeamNotExist{args: map[string]any{"orgID": orgID, "name": name}}
	} else if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTeamOfOrgByName returns team by given team name and organization.
func GetTeamOfOrgByName(orgID int64, name string) (*Team, error) {
	return getTeamOfOrgByName(db, orgID, name)
}

func getTeamByID(tx *gorm.DB, teamID int64) (*Team, error) {
	t := new(Team)
	err := tx.Where("id = ?", teamID).First(t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTeamNotExist{args: map[string]any{"teamID": teamID}}
	} else if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTeamByID returns team by given ID.
func GetTeamByID(teamID int64) (*Team, error) {
	return getTeamByID(db, teamID)
}

func getTeamsByOrgID(tx *gorm.DB, orgID int64) ([]*Team, error) {
	teams := make([]*Team, 0, 3)
	return teams, tx.Where("org_id = ?", orgID).Find(&teams).Error
}

// GetTeamsByOrgID returns all teams belong to given organization.
func GetTeamsByOrgID(orgID int64) ([]*Team, error) {
	return getTeamsByOrgID(db, orgID)
}

// UpdateTeam updates information of team.
func UpdateTeam(t *Team, authChanged bool) (err error) {
	if t.Name == "" {
		return errors.New("empty team name")
	}

	if len(t.Description) > 255 {
		t.Description = t.Description[:255]
	}

	t.LowerName = strings.ToLower(t.Name)
	existingTeam := new(Team)
	err = db.Where("org_id = ? AND lower_name = ? AND id != ?", t.OrgID, t.LowerName, t.ID).First(existingTeam).Error
	if err == nil {
		return ErrTeamAlreadyExist{existingTeam.ID, t.OrgID, t.LowerName}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Team{}).Where("id = ?", t.ID).Updates(t).Error; err != nil {
			return errors.Newf("update: %v", err)
		}

		// Update access for team members if needed.
		if authChanged {
			if err := t.getRepositories(tx); err != nil {
				return errors.Newf("getRepositories:%v", err)
			}

			for _, repo := range t.Repos {
				if err := repo.recalculateTeamAccesses(tx, 0); err != nil {
					return errors.Newf("recalculateTeamAccesses: %v", err)
				}
			}
		}

		return nil
	})
}

// DeleteTeam deletes given team.
// It's caller's responsibility to assign organization ID.
func DeleteTeam(t *Team) error {
	if err := t.GetRepositories(); err != nil {
		return err
	}

	// Get organization.
	org, err := Handle.Users().GetByID(context.TODO(), t.OrgID)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Delete all accesses.
		for _, repo := range t.Repos {
			if err := repo.recalculateTeamAccesses(tx, t.ID); err != nil {
				return err
			}
		}

		// Delete team-user.
		if err := tx.Where("org_id = ? AND team_id = ?", org.ID, t.ID).Delete(new(TeamUser)).Error; err != nil {
			return err
		}

		// Delete team.
		if err := tx.Where("id = ?", t.ID).Delete(new(Team)).Error; err != nil {
			return err
		}
		// Update organization number of teams.
		return tx.Exec("UPDATE `user` SET num_teams=num_teams-1 WHERE id = ?", t.OrgID).Error
	})
}

// ___________                    ____ ___
// \__    ___/___ _____    _____ |    |   \______ ___________
//   |    |_/ __ \\__  \  /     \|    |   /  ___// __ \_  __ \
//   |    |\  ___/ / __ \|  Y Y  \    |  /\___ \\  ___/|  | \/
//   |____| \___  >____  /__|_|  /______//____  >\___  >__|
//              \/     \/      \/             \/     \/

// TeamUser represents an team-user relation.
type TeamUser struct {
	ID     int64
	OrgID  int64 `gorm:"index"`
	TeamID int64 `gorm:"uniqueIndex:team_user_team_id_uid"`
	UID    int64 `gorm:"uniqueIndex:team_user_team_id_uid"`
}

func isTeamMember(tx *gorm.DB, orgID, teamID, uid int64) bool {
	err := tx.Where("org_id = ? AND team_id = ? AND uid = ?", orgID, teamID, uid).First(new(TeamUser)).Error
	return err == nil
}

// IsTeamMember returns true if given user is a member of team.
func IsTeamMember(orgID, teamID, uid int64) bool {
	return isTeamMember(db, orgID, teamID, uid)
}

func getTeamMembers(tx *gorm.DB, teamID int64) (_ []*User, err error) {
	teamUsers := make([]*TeamUser, 0, 10)
	if err = tx.Select("id, org_id, team_id, uid").Where("team_id = ?", teamID).Find(&teamUsers).Error; err != nil {
		return nil, errors.Newf("get team-users: %v", err)
	}
	members := make([]*User, 0, len(teamUsers))
	for i := range teamUsers {
		member := new(User)
		if err = tx.Where("id = ?", teamUsers[i].UID).First(member).Error; err != nil {
			return nil, errors.Newf("get user '%d': %v", teamUsers[i].UID, err)
		}
		members = append(members, member)
	}
	return members, nil
}

// GetTeamMembers returns all members in given team of organization.
func GetTeamMembers(teamID int64) ([]*User, error) {
	return getTeamMembers(db, teamID)
}

func getUserTeams(tx *gorm.DB, orgID, userID int64) ([]*Team, error) {
	teamUsers := make([]*TeamUser, 0, 5)
	if err := tx.Where("uid = ? AND org_id = ?", userID, orgID).Find(&teamUsers).Error; err != nil {
		return nil, err
	}

	teamIDs := make([]int64, len(teamUsers)+1)
	for i := range teamUsers {
		teamIDs[i] = teamUsers[i].TeamID
	}
	teamIDs[len(teamUsers)] = -1

	teams := make([]*Team, 0, len(teamIDs))
	return teams, tx.Where("org_id = ? AND id IN ?", orgID, teamIDs).Find(&teams).Error
}

// GetUserTeams returns all teams that user belongs to in given organization.
func GetUserTeams(orgID, userID int64) ([]*Team, error) {
	return getUserTeams(db, orgID, userID)
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

	return db.Transaction(func(tx *gorm.DB) error {
		tu := &TeamUser{
			UID:    userID,
			OrgID:  orgID,
			TeamID: teamID,
		}
		if err := tx.Create(tu).Error; err != nil {
			return err
		}
		if err := tx.Model(&Team{}).Where("id = ?", t.ID).Updates(t).Error; err != nil {
			return err
		}

		// Give access to team repositories.
		for _, repo := range t.Repos {
			if err := repo.recalculateTeamAccesses(tx, 0); err != nil {
				return err
			}
		}

		// We make sure it exists before.
		ou := new(OrgUser)
		if err := tx.Where("uid = ? AND org_id = ?", userID, orgID).First(ou).Error; err != nil {
			return err
		}
		ou.NumTeams++
		if t.IsOwnerTeam() {
			ou.IsOwner = true
		}
		return tx.Model(&OrgUser{}).Where("id = ?", ou.ID).Updates(ou).Error
	})
}

func removeTeamMember(tx *gorm.DB, orgID, teamID, uid int64) error {
	if !isTeamMember(tx, orgID, teamID, uid) {
		return nil
	}

	// Get team and its repositories.
	t, err := getTeamByID(tx, teamID)
	if err != nil {
		return err
	}

	// Check if the user to delete is the last member in owner team.
	if t.IsOwnerTeam() && t.NumMembers == 1 {
		return ErrLastOrgOwner{UID: uid}
	}

	t.NumMembers--

	if err = t.getRepositories(tx); err != nil {
		return err
	}

	// Get organization.
	org, err := getUserByID(tx, orgID)
	if err != nil {
		return err
	}

	tu := &TeamUser{
		UID:    uid,
		OrgID:  orgID,
		TeamID: teamID,
	}
	if err := tx.Where("uid = ? AND org_id = ? AND team_id = ?", uid, orgID, teamID).Delete(tu).Error; err != nil {
		return err
	}
	if err = tx.Model(&Team{}).Where("id = ?", t.ID).Updates(t).Error; err != nil {
		return err
	}

	// Delete access to team repositories.
	for _, repo := range t.Repos {
		if err = repo.recalculateTeamAccesses(tx, 0); err != nil {
			return err
		}
	}

	// This must exist.
	ou := new(OrgUser)
	if err = tx.Where("uid = ? AND org_id = ?", uid, org.ID).First(ou).Error; err != nil {
		return err
	}
	ou.NumTeams--
	if t.IsOwnerTeam() {
		ou.IsOwner = false
	}
	return tx.Model(&OrgUser{}).Where("id = ?", ou.ID).Updates(ou).Error
}

// RemoveTeamMember removes member from given team of given organization.
func RemoveTeamMember(orgID, teamID, uid int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return removeTeamMember(tx, orgID, teamID, uid)
	})
}

// ___________                  __________
// \__    ___/___ _____    _____\______   \ ____ ______   ____
//   |    |_/ __ \\__  \  /     \|       _// __ \\____ \ /  _ \
//   |    |\  ___/ / __ \|  Y Y  \    |   \  ___/|  |_> >  <_> )
//   |____| \___  >____  /__|_|  /____|_  /\___  >   __/ \____/
//              \/     \/      \/       \/     \/|__|

// TeamRepo represents an team-repository relation.
type TeamRepo struct {
	ID     int64
	OrgID  int64 `gorm:"index"`
	TeamID int64 `gorm:"uniqueIndex:team_repo_team_id_repo_id"`
	RepoID int64 `gorm:"uniqueIndex:team_repo_team_id_repo_id"`
}

func hasTeamRepo(tx *gorm.DB, orgID, teamID, repoID int64) bool {
	err := tx.Where("org_id = ? AND team_id = ? AND repo_id = ?", orgID, teamID, repoID).First(new(TeamRepo)).Error
	return err == nil
}

// HasTeamRepo returns true if given team has access to the repository of the organization.
func HasTeamRepo(orgID, teamID, repoID int64) bool {
	return hasTeamRepo(db, orgID, teamID, repoID)
}

func addTeamRepo(tx *gorm.DB, orgID, teamID, repoID int64) error {
	return tx.Create(&TeamRepo{
		OrgID:  orgID,
		TeamID: teamID,
		RepoID: repoID,
	}).Error
}

// AddTeamRepo adds new repository relation to team.
func AddTeamRepo(orgID, teamID, repoID int64) error {
	return addTeamRepo(db, orgID, teamID, repoID)
}

func removeTeamRepo(tx *gorm.DB, teamID, repoID int64) error {
	return tx.Where("team_id = ? AND repo_id = ?", teamID, repoID).Delete(new(TeamRepo)).Error
}

// RemoveTeamRepo deletes repository relation to team.
func RemoveTeamRepo(teamID, repoID int64) error {
	return removeTeamRepo(db, teamID, repoID)
}

// GetTeamsHaveAccessToRepo returns all teams in an organization that have given access level to the repository.
func GetTeamsHaveAccessToRepo(orgID, repoID int64, mode AccessMode) ([]*Team, error) {
	teams := make([]*Team, 0, 5)
	return teams, db.Table("team").
		Where("team.authorize >= ?", mode).
		Joins("INNER JOIN team_repo ON team_repo.team_id = team.id").
		Where("team_repo.org_id = ? AND team_repo.repo_id = ?", orgID, repoID).
		Find(&teams).Error
}
