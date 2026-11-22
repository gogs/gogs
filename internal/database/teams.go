package database

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
)

// TeamsStore is the storage layer for teams of organizations, their members
// and their repositories.
type TeamsStore struct {
	db *gorm.DB
}

func newTeamsStore(db *gorm.DB) *TeamsStore {
	return &TeamsStore{db: db}
}

// Create creates the team in its organization. The team must have a name and
// an organization ID, and no other team of the organization may have the same
// name, ignoring case.
func (s *TeamsStore) Create(ctx context.Context, t *Team) error {
	if t.Name == "" {
		return errors.New("empty team name")
	} else if t.OrgID == 0 {
		return errors.New("OrgID is not assigned")
	}

	if err := IsUsableTeamName(t.Name); err != nil {
		return err
	}

	db := s.db.WithContext(ctx)
	var count int64
	if err := db.Model(new(User)).Where("id = ?", t.OrgID).Count(&count).Error; err != nil {
		return errors.Wrap(err, "count organizations")
	} else if count == 0 {
		return ErrOrgNotExist
	}

	t.LowerName = strings.ToLower(t.Name)
	existing, err := s.GetByName(ctx, t.OrgID, t.LowerName)
	if err == nil {
		return ErrTeamAlreadyExist{existing.ID, t.OrgID, t.LowerName}
	} else if !IsErrTeamNotExist(err) {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return errors.Wrap(err, "create")
		}
		return tx.Model(new(User)).Where("id = ?", t.OrgID).UpdateColumn("num_teams", gorm.Expr("num_teams + 1")).Error
	})
}

func (s *TeamsStore) get(ctx context.Context, args map[string]any, query string, values ...any) (*Team, error) {
	t := new(Team)
	err := s.db.WithContext(ctx).Where(query, values...).First(t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTeamNotExist{args: args}
		}
		return nil, err
	}
	return t, nil
}

// GetByID returns the team with given ID. It returns ErrTeamNotExist when not
// found.
func (s *TeamsStore) GetByID(ctx context.Context, id int64) (*Team, error) {
	return s.get(ctx, map[string]any{"teamID": id}, "id = ?", id)
}

// GetByName returns the team of the organization with given ID that has given
// name, ignoring case. It returns ErrTeamNotExist when not found.
func (s *TeamsStore) GetByName(ctx context.Context, orgID int64, name string) (*Team, error) {
	return s.get(ctx, map[string]any{"orgID": orgID, "name": name}, "org_id = ? AND lower_name = ?", orgID, strings.ToLower(name))
}

// GetOwnerTeam returns the owner team of the organization with given ID.
func (s *TeamsStore) GetOwnerTeam(ctx context.Context, orgID int64) (*Team, error) {
	return s.GetByName(ctx, orgID, ownerTeamName)
}

// ListByOrg returns the teams of the organization with given ID, in the order
// they were created.
func (s *TeamsStore) ListByOrg(ctx context.Context, orgID int64) ([]*Team, error) {
	teams := make([]*Team, 0, 3)
	return teams, s.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id ASC").Find(&teams).Error
}

// ListByMember returns the teams of the organization with given ID that the
// user with given ID is a member of, in the order they were created.
func (s *TeamsStore) ListByMember(ctx context.Context, orgID, userID int64) ([]*Team, error) {
	teams := make([]*Team, 0, 3)
	return teams, s.db.WithContext(ctx).Select("team.*").
		Joins("INNER JOIN team_user ON team_user.team_id = team.id").
		Where("team_user.org_id = ? AND team_user.uid = ?", orgID, userID).
		Order("team.id ASC").
		Find(&teams).Error
}

// ListByRepo returns the teams of the organization with given ID that have the
// repository with given ID and at least given access mode, in the order they
// were created.
func (s *TeamsStore) ListByRepo(ctx context.Context, orgID, repoID int64, mode AccessMode) ([]*Team, error) {
	teams := make([]*Team, 0, 5)
	return teams, s.db.WithContext(ctx).Select("team.*").
		Joins("INNER JOIN team_repo ON team_repo.team_id = team.id").
		Where("team.authorize >= ? AND team_repo.org_id = ? AND team_repo.repo_id = ?", mode, orgID, repoID).
		Order("team.id ASC").
		Find(&teams).Error
}

// Update saves the team, whose name must not be used by another team of its
// organization, and recalculates the accesses of its repositories when its
// access mode changed. Descriptions longer than 255 bytes are cut.
func (s *TeamsStore) Update(ctx context.Context, t *Team, authChanged bool) error {
	if t.Name == "" {
		return errors.New("empty team name")
	}

	if len(t.Description) > 255 {
		t.Description = t.Description[:255]
	}

	db := s.db.WithContext(ctx)
	t.LowerName = strings.ToLower(t.Name)
	existing := new(Team)
	err := db.Where("org_id = ? AND lower_name = ? AND id != ?", t.OrgID, t.LowerName, t.ID).First(existing).Error
	if err == nil {
		return ErrTeamAlreadyExist{existing.ID, t.OrgID, t.LowerName}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrap(err, "get team of same name")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(t).Select("*").Updates(t).Error; err != nil {
			return errors.Wrap(err, "update")
		}
		if !authChanged {
			return nil
		}

		repos, err := newTeamsStore(tx).ListRepos(ctx, t.ID)
		if err != nil {
			return errors.Wrap(err, "list repositories")
		}
		for _, repo := range repos {
			if err = newPermissionsStore(tx).recalculateTeamAccesses(ctx, repo, 0); err != nil {
				return errors.Wrapf(err, "recalculate accesses of repository %d", repo.ID)
			}
		}
		return nil
	})
}

// Delete deletes the team and its memberships, and recalculates the accesses
// of its repositories.
func (s *TeamsStore) Delete(ctx context.Context, t *Team) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The organization is locked before the team, the order adding a
		// member locks them in, so the two cannot deadlock.
		err := tx.Model(new(User)).Where("id = ?", t.OrgID).UpdateColumn("num_teams", gorm.Expr("num_teams - 1")).Error
		if err != nil {
			return errors.Wrap(err, "count teams")
		}

		repos, err := newTeamsStore(tx).ListRepos(ctx, t.ID)
		if err != nil {
			return errors.Wrap(err, "list repositories")
		}
		for _, repo := range repos {
			if err = newPermissionsStore(tx).recalculateTeamAccesses(ctx, repo, t.ID); err != nil {
				return errors.Wrapf(err, "recalculate accesses of repository %d", repo.ID)
			}
		}

		if err = tx.Where("org_id = ? AND team_id = ?", t.OrgID, t.ID).Delete(new(TeamUser)).Error; err != nil {
			return errors.Wrap(err, "delete members")
		}
		if err = tx.Where("team_id = ?", t.ID).Delete(new(TeamRepo)).Error; err != nil {
			return errors.Wrap(err, "delete repositories")
		}
		if err = tx.Where("id = ?", t.ID).Delete(new(Team)).Error; err != nil {
			return errors.Wrap(err, "delete")
		}
		return nil
	})
}

// IsMember returns true if the user with given ID is a member of the team with
// given ID of the organization with given ID.
func (s *TeamsStore) IsMember(ctx context.Context, orgID, teamID, userID int64) bool {
	var count int64
	s.db.WithContext(ctx).Model(new(TeamUser)).Where("org_id = ? AND team_id = ? AND uid = ?", orgID, teamID, userID).Count(&count)
	return count > 0
}

// ListMembers returns the members of the team with given ID, in the order they
// joined. A member who no longer exists is an empty user.
func (s *TeamsStore) ListMembers(ctx context.Context, teamID int64) ([]*User, error) {
	db := s.db.WithContext(ctx)
	teamUsers := make([]*TeamUser, 0, 10)
	if err := db.Where("team_id = ?", teamID).Order("id ASC").Find(&teamUsers).Error; err != nil {
		return nil, errors.Wrap(err, "list team users")
	}

	members := make([]*User, 0, len(teamUsers))
	for _, tu := range teamUsers {
		member := new(User)
		if err := db.Where("id = ?", tu.UID).Limit(1).Find(member).Error; err != nil {
			return nil, errors.Wrapf(err, "get user %d", tu.UID)
		}
		members = append(members, member)
	}
	return members, nil
}

// errTeamReposChanged is returned when a repository was added to the team
// after the repositories of the team were locked.
var errTeamReposChanged = errors.New("repositories of the team changed")

// AddMember adds the user with given ID to the team with given ID of the
// organization with given ID, and to the organization when the user is not a
// member yet. Adding a member that the team has is a no-op.
func (s *TeamsStore) AddMember(ctx context.Context, orgID, teamID, userID int64) error {
	if s.IsMember(ctx, orgID, teamID, userID) {
		return nil
	}

	// A repository added to the team meanwhile is locked by the next attempt.
	for attempt := 1; ; attempt++ {
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return newTeamsStore(tx).addMember(ctx, orgID, teamID, userID)
		}, readCommitted()...)
		if !errors.Is(err, errTeamReposChanged) || attempt == 3 {
			return err
		}
	}
}

// addMember is AddMember without a transaction of its own and checks.
func (s *TeamsStore) addMember(ctx context.Context, orgID, teamID, userID int64) error {
	db := s.db.WithContext(ctx)
	// Lock the repositories of the team before rows of the organization and
	// the team, as other transactions do, so the repositories cannot be
	// deleted or transferred until the member has their accesses.
	var lockedIDs []int64
	err := db.Model(new(TeamRepo)).Where("team_id = ?", teamID).Order("repo_id ASC").Pluck("repo_id", &lockedIDs).Error
	if err != nil {
		return errors.Wrap(err, "list team repositories")
	}
	// Locking reads see the latest rows even when other reads come from an
	// older snapshot, e.g., at REPEATABLE READ on MySQL, so only repositories
	// that still belong to the organization and the team get accesses.
	ownedIDs := make([]int64, 0, len(lockedIDs))
	for _, repoID := range lockedIDs {
		ownerID, exists, err := lockRepository(s.db, repoID)
		if err != nil {
			return err
		} else if !exists || ownerID != orgID {
			continue
		}

		query := db.Model(new(TeamRepo)).Where("team_id = ? AND repo_id = ?", teamID, repoID)
		if !conf.UseSQLite3 {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var ids []int64
		if err = query.Pluck("id", &ids).Error; err != nil {
			return errors.Wrap(err, "lock team repository")
		} else if len(ids) > 0 {
			ownedIDs = append(ownedIDs, repoID)
		}
	}

	if err := newOrganizationsStore(s.db).addMember(ctx, orgID, userID); err != nil {
		return errors.Wrap(err, "add organization member")
	}

	t, err := s.GetByID(ctx, teamID)
	if err != nil {
		return err
	}
	err = db.Create(&TeamUser{UID: userID, OrgID: orgID, TeamID: teamID}).Error
	if err != nil {
		return errors.Wrap(err, "create team user")
	}
	err = db.Model(t).UpdateColumn("num_members", gorm.Expr("num_members + 1")).Error
	if err != nil {
		return errors.Wrap(err, "count members")
	}

	// Adding, removing, deleting or transferring a repository of the team
	// updates the team too, so with the team locked, the repositories of the
	// team no longer change until the member is added. One added since the
	// others were locked can only be locked before the team.
	var repoIDs []int64
	err = db.Model(new(TeamRepo)).Where("team_id = ?", teamID).Pluck("repo_id", &repoIDs).Error
	if err != nil {
		return errors.Wrap(err, "list team repositories")
	}
	for _, repoID := range repoIDs {
		if !slices.Contains(lockedIDs, repoID) {
			return errTeamReposChanged
		}
	}
	for _, repoID := range repoIDs {
		if !slices.Contains(ownedIDs, repoID) {
			continue
		}
		repo, err := newReposStore(s.db).getWithAttributes(ctx, repoID)
		if err != nil {
			return errors.Wrapf(err, "get repository %d", repoID)
		}
		if err = newPermissionsStore(s.db).recalculateTeamAccesses(ctx, repo, 0); err != nil {
			return errors.Wrapf(err, "recalculate accesses of repository %d", repo.ID)
		}
	}

	updates := map[string]any{"num_teams": gorm.Expr("num_teams + 1")}
	if t.IsOwnerTeam() {
		updates["is_owner"] = true
	}
	return db.Model(new(OrgUser)).Where("uid = ? AND org_id = ?", userID, orgID).UpdateColumns(updates).Error
}

// RemoveMember removes the user with given ID from the team with given ID of
// the organization with given ID, and recalculates the accesses of the
// repositories of the team. The user stays a member of the organization. It
// returns ErrLastOrgOwner when the user is the last member of the owner team.
// Removing a user who is not a member is a no-op.
func (s *TeamsStore) RemoveMember(ctx context.Context, orgID, teamID, userID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newTeamsStore(tx).removeMember(ctx, orgID, teamID, userID)
	})
}

// removeMember is RemoveMember without a transaction of its own.
func (s *TeamsStore) removeMember(ctx context.Context, orgID, teamID, userID int64) error {
	if !s.IsMember(ctx, orgID, teamID, userID) {
		return nil
	}

	t, err := s.GetByID(ctx, teamID)
	if err != nil {
		return err
	} else if t.IsOwnerTeam() && t.NumMembers == 1 {
		return ErrLastOrgOwner{UID: userID}
	}

	db := s.db.WithContext(ctx)
	err = db.Where("uid = ? AND org_id = ? AND team_id = ?", userID, orgID, teamID).Delete(new(TeamUser)).Error
	if err != nil {
		return errors.Wrap(err, "delete team user")
	}
	err = db.Model(t).UpdateColumn("num_members", gorm.Expr("num_members - 1")).Error
	if err != nil {
		return errors.Wrap(err, "count members")
	}

	repos, err := s.ListRepos(ctx, t.ID)
	if err != nil {
		return errors.Wrap(err, "list repositories")
	}
	for _, repo := range repos {
		if err = newPermissionsStore(s.db).recalculateTeamAccesses(ctx, repo, 0); err != nil {
			return errors.Wrapf(err, "recalculate accesses of repository %d", repo.ID)
		}
	}

	updates := map[string]any{"num_teams": gorm.Expr("num_teams - 1")}
	if t.IsOwnerTeam() {
		updates["is_owner"] = false
	}
	return db.Model(new(OrgUser)).Where("uid = ? AND org_id = ?", userID, orgID).UpdateColumns(updates).Error
}

// HasRepo returns true if the team with given ID of the organization with
// given ID has the repository with given ID.
func (s *TeamsStore) HasRepo(ctx context.Context, orgID, teamID, repoID int64) bool {
	var count int64
	s.db.WithContext(ctx).Model(new(TeamRepo)).Where("org_id = ? AND team_id = ? AND repo_id = ?", orgID, teamID, repoID).Count(&count)
	return count > 0
}

// ListRepos returns the repositories of the team with given ID, in the order
// they were added.
func (s *TeamsStore) ListRepos(ctx context.Context, teamID int64) ([]*Repository, error) {
	db := s.db.WithContext(ctx)
	var repoIDs []int64
	err := db.Model(new(TeamRepo)).Where("team_id = ?", teamID).Order("id ASC").Pluck("repo_id", &repoIDs).Error
	if err != nil {
		return nil, errors.Wrap(err, "list team repositories")
	}

	repos := make([]*Repository, 0, len(repoIDs))
	for _, repoID := range repoIDs {
		repo, err := newReposStore(db).getWithAttributes(ctx, repoID)
		if err != nil {
			return nil, errors.Wrapf(err, "get repository %d", repoID)
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// AddRepo adds the repository, which must belong to the organization of the
// team, to the team, recalculates the accesses of the repository, and makes
// the members of the team watch it. Adding a repository that the team has is
// a no-op.
func (s *TeamsStore) AddRepo(ctx context.Context, t *Team, repo *Repository) error {
	if repo.OwnerID != t.OrgID {
		return errors.New("repository does not belong to organization")
	} else if s.HasRepo(ctx, t.OrgID, t.ID, repo.ID) {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The repository may have been deleted or transferred while waiting
		// for it.
		if ownerID, exists, err := lockRepository(tx, repo.ID); err != nil {
			return err
		} else if !exists {
			return ErrRepoNotExist{args: errx.Args{"repoID": repo.ID}}
		} else if ownerID != t.OrgID {
			return errors.New("repository does not belong to organization")
		}
		return newTeamsStore(tx).addRepo(ctx, t, repo)
	})
}

// addRepo is AddRepo without a transaction of its own and checks.
func (s *TeamsStore) addRepo(ctx context.Context, t *Team, repo *Repository) error {
	db := s.db.WithContext(ctx)
	err := db.Create(&TeamRepo{OrgID: t.OrgID, TeamID: t.ID, RepoID: repo.ID}).Error
	if err != nil {
		return errors.Wrap(err, "create team repository")
	}
	err = db.Model(t).UpdateColumn("num_repos", gorm.Expr("num_repos + 1")).Error
	if err != nil {
		return errors.Wrap(err, "count repositories")
	}
	t.NumRepos++

	if err = newPermissionsStore(s.db).recalculateTeamAccesses(ctx, repo, 0); err != nil {
		return errors.Wrap(err, "recalculate accesses")
	}

	members, err := s.ListMembers(ctx, t.ID)
	if err != nil {
		return errors.Wrap(err, "list members")
	}
	for _, member := range members {
		if err = newReposStore(s.db).watch(ctx, member.ID, repo.ID); err != nil {
			return errors.Wrapf(err, "watch for user %d", member.ID)
		}
	}
	return nil
}

// RemoveRepo removes the repository with given ID from the team, recalculates
// the accesses of the repository, and makes the members of the team who can
// no longer read it stop watching it. Removing a repository that the team
// does not have is a no-op.
func (s *TeamsStore) RemoveRepo(ctx context.Context, t *Team, repoID int64) error {
	if !s.HasRepo(ctx, t.OrgID, t.ID, repoID) {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The repository may have been deleted or left the team while waiting
		// for it, which removes it from the team too.
		if _, exists, err := lockRepository(tx, repoID); err != nil {
			return err
		} else if !exists || !newTeamsStore(tx).HasRepo(ctx, t.OrgID, t.ID, repoID) {
			return nil
		}
		repo, err := newReposStore(tx).getWithAttributes(ctx, repoID)
		if err != nil {
			return errors.Wrap(err, "get repository")
		}
		return newTeamsStore(tx).removeRepo(ctx, t, repo, true)
	})
}

// removeRepo is RemoveRepo without a transaction of its own and checks. The
// accesses are only recalculated when asked, which deleting the repository
// does not need.
func (s *TeamsStore) removeRepo(ctx context.Context, t *Team, repo *Repository, recalculate bool) error {
	db := s.db.WithContext(ctx)
	err := db.Where("team_id = ? AND repo_id = ?", t.ID, repo.ID).Delete(new(TeamRepo)).Error
	if err != nil {
		return errors.Wrap(err, "delete team repository")
	}
	err = db.Model(t).UpdateColumn("num_repos", gorm.Expr("num_repos - 1")).Error
	if err != nil {
		return errors.Wrap(err, "count repositories")
	}
	t.NumRepos--

	if recalculate {
		if err = newPermissionsStore(s.db).recalculateTeamAccesses(ctx, repo, t.ID); err != nil {
			return errors.Wrap(err, "recalculate accesses")
		}
	}

	members, err := s.ListMembers(ctx, t.ID)
	if err != nil {
		return errors.Wrap(err, "list members")
	}
	for _, member := range members {
		mode := AccessModeNone
		// Everyone has read access to public repository.
		if !repo.IsPrivate {
			mode = AccessModeRead
		}
		if member.ID > 0 && member.ID == repo.OwnerID {
			mode = AccessModeOwner
		} else if member.ID > 0 {
			access := new(Access)
			err = db.Where("user_id = ? AND repo_id = ?", member.ID, repo.ID).First(access).Error
			if err == nil {
				mode = access.Mode
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.Wrapf(err, "get access of user %d", member.ID)
			}
		}
		if mode >= AccessModeRead {
			continue
		}

		if err = newReposStore(s.db).unwatch(ctx, member.ID, repo.ID); err != nil {
			return errors.Wrapf(err, "unwatch for user %d", member.ID)
		}
	}
	return nil
}

const ownerTeamName = "Owners"

// Team represents an organization team.
type Team struct {
	ID          int64
	OrgID       int64      `gorm:"index:IDX_team_org_id"`
	LowerName   string     `gorm:"size:255"`
	Name        string     `gorm:"size:255"`
	Description string     `gorm:"size:255"`
	Authorize   AccessMode `gorm:"size:32"`
	NumRepos    int        `gorm:"size:32"`
	NumMembers  int        `gorm:"size:32"`
}

// AfterFind implements the GORM query hook.
func (t *Team) AfterFind(_ *gorm.DB) error {
	// LEGACY [1.0]: this is backward compatibility bug fix for https://github.com/gogs/gogs/issues/3671
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

var reservedTeamNames = map[string]struct{}{
	"new": {},
}

// IsUsableTeamName returns an error if given name is a reserved name or pattern.
func IsUsableTeamName(name string) error {
	return isNameAllowed(reservedTeamNames, nil, name)
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

// TeamUser represents a team-user relation.
type TeamUser struct {
	ID     int64
	OrgID  int64 `gorm:"index:IDX_team_user_org_id"`
	TeamID int64 `gorm:"uniqueIndex:UQE_team_user_s"`
	UID    int64 `gorm:"uniqueIndex:UQE_team_user_s"`
}

// TeamRepo represents a team-repository relation.
type TeamRepo struct {
	ID     int64
	OrgID  int64 `gorm:"index:IDX_team_repo_org_id"`
	TeamID int64 `gorm:"uniqueIndex:UQE_team_repo_s"`
	RepoID int64 `gorm:"uniqueIndex:UQE_team_repo_s"`
}
