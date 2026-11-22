package database

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbx"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/userx"
)

// OrganizationsStore is the storage layer for organizations.
type OrganizationsStore struct {
	db *gorm.DB
}

func newOrganizationsStore(db *gorm.DB) *OrganizationsStore {
	return &OrganizationsStore{db: db}
}

type ListOrgsOptions struct {
	// Filter by the membership with the given user ID.
	MemberID int64
	// Whether to include private memberships.
	IncludePrivateMembers bool
}

// List returns a list of organizations filtered by options.
func (s *OrganizationsStore) List(ctx context.Context, opts ListOrgsOptions) ([]*Organization, error) {
	if opts.MemberID <= 0 {
		return nil, errors.New("MemberID must be greater than 0")
	}

	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "org"
		JOIN org_user ON org_user.org_id = org.id
		WHERE
			org_user.uid = @memberID
		[AND org_user.is_public = @includePrivateMembers]
		ORDER BY org.id ASC
	*/
	tx := s.db.WithContext(ctx).
		Joins(dbx.Quote("JOIN org_user ON org_user.org_id = %s.id", "user")).
		Where("org_user.uid = ?", opts.MemberID).
		Order(dbx.Quote("%s.id ASC", "user"))
	if !opts.IncludePrivateMembers {
		tx = tx.Where("org_user.is_public = ?", true)
	}

	var orgs []*Organization
	return orgs, tx.Find(&orgs).Error
}

// SearchByName returns a list of organizations whose username or full name
// matches the given keyword case-insensitively. Results are paginated by given
// page and page size, and sorted by the given order (e.g., "id DESC"). A total
// count of all results is also returned. If the order is not given, it's up to
// the database to decide.
func (s *OrganizationsStore) SearchByName(ctx context.Context, keyword string, page, pageSize int, orderBy string) ([]*Organization, int64, error) {
	return searchUserByName(ctx, s.db, UserTypeOrganization, keyword, page, pageSize, orderBy)
}

// ErrOrgOwnerIsOrganization is returned when an organization is given as the
// owner of a new organization, which only users can own.
type ErrOrgOwnerIsOrganization struct {
	args errx.Args
}

// IsErrOrgOwnerIsOrganization returns true if the underlying error has the type
// ErrOrgOwnerIsOrganization.
func IsErrOrgOwnerIsOrganization(err error) bool {
	return errors.As(err, &ErrOrgOwnerIsOrganization{})
}

func (err ErrOrgOwnerIsOrganization) Error() string {
	return fmt.Sprintf("organization cannot own an organization: %v", err.args)
}

// Create creates the organization with the owner as its first member, and
// the owner team of the organization with the owner in it. It returns
// ErrOrgOwnerIsOrganization when the owner is an organization.
func (s *OrganizationsStore) Create(ctx context.Context, org, owner *User) (err error) {
	if owner.IsOrganization() {
		return ErrOrgOwnerIsOrganization{args: errx.Args{"ownerID": owner.ID}}
	}
	if err = isUsernameAllowed(org.Name); err != nil {
		return err
	}

	if newUsersStore(s.db).IsUsernameUsed(ctx, org.Name, 0) {
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
	org.Type = UserTypeOrganization
	org.MaxRepoCreation = -1
	org.NumTeams = 1
	org.NumMembers = 1

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(org).Error; err != nil {
			return errors.Wrap(err, "create organization")
		}

		err := tx.Create(&OrgUser{
			UID:      owner.ID,
			OrgID:    org.ID,
			IsOwner:  true,
			NumTeams: 1,
		}).Error
		if err != nil {
			return errors.Wrap(err, "create organization user")
		}

		t := &Team{
			OrgID:      org.ID,
			LowerName:  strings.ToLower(ownerTeamName),
			Name:       ownerTeamName,
			Authorize:  AccessModeOwner,
			NumMembers: 1,
		}
		if err = tx.Create(t).Error; err != nil {
			return errors.Wrap(err, "create owner team")
		}

		err = tx.Create(&TeamUser{
			UID:    owner.ID,
			OrgID:  org.ID,
			TeamID: t.ID,
		}).Error
		if err != nil {
			return errors.Wrap(err, "create team user")
		}

		if err = os.MkdirAll(repox.UserPath(org.Name), os.ModePerm); err != nil {
			return errors.Wrap(err, "create directory")
		}
		return nil
	})
}

// GetByName returns the organization with given name, ignoring case. It
// returns ErrOrgNotExist when not found.
func (s *OrganizationsStore) GetByName(ctx context.Context, name string) (*Organization, error) {
	if name == "" {
		return nil, ErrOrgNotExist
	}

	org := new(Organization)
	err := s.db.WithContext(ctx).Where("lower_name = ? AND type = ?", strings.ToLower(name), UserTypeOrganization).First(org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrgNotExist
		}
		return nil, err
	}
	return org, nil
}

// Count returns the number of organizations.
func (s *OrganizationsStore) Count(ctx context.Context) int64 {
	var count int64
	s.db.WithContext(ctx).Model(new(User)).Where("type = ?", UserTypeOrganization).Count(&count)
	return count
}

// ListByPage returns the organizations of given page, in the order they were
// created.
func (s *OrganizationsStore) ListByPage(ctx context.Context, page, pageSize int) ([]*Organization, error) {
	orgs := make([]*Organization, 0, pageSize)
	return orgs, s.db.WithContext(ctx).
		Where("type = ?", UserTypeOrganization).
		Order("id ASC").
		Limit(pageSize).Offset((max(page, 1) - 1) * pageSize).
		Find(&orgs).Error
}

// Delete deletes the organization, which must have no repositories, with its
// teams and memberships.
func (s *OrganizationsStore) Delete(ctx context.Context, org *Organization) error {
	// A column with 0 means "none" in some tables, e.g., webhooks of
	// repositories have org_id 0, so deleting by a non-positive ID would hit
	// rows of no organization.
	if org.ID <= 0 {
		return errors.Newf("invalid organization ID %d", org.ID)
	}

	err := newUsersStore(s.db).DeleteByID(ctx, org.ID, false)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Deliveries for repositories that have been transferred out are reachable
		// only through the webhooks, so they go with them.
		err := tx.Where("hook_id IN (?)", tx.Model(new(Webhook)).Select("id").Where("org_id = ?", org.ID)).Delete(new(HookTask)).Error
		if err != nil {
			return errors.Wrap(err, "delete deliveries of webhooks")
		}
		for _, table := range []any{new(TeamRepo), new(TeamUser), new(Team), new(OrgUser), new(Webhook)} {
			if err := tx.Where("org_id = ?", org.ID).Delete(table).Error; err != nil {
				return errors.Wrapf(err, "delete %T", table)
			}
		}
		return nil
	})
}

// IsOwner returns true if the user with given ID is an owner of the
// organization with given ID.
func (s *OrganizationsStore) IsOwner(ctx context.Context, orgID, userID int64) bool {
	var count int64
	s.db.WithContext(ctx).Model(new(OrgUser)).Where("is_owner = ? AND uid = ? AND org_id = ?", true, userID, orgID).Count(&count)
	return count > 0
}

// IsMember returns true if the user with given ID is a member of the
// organization with given ID.
func (s *OrganizationsStore) IsMember(ctx context.Context, orgID, userID int64) bool {
	var count int64
	s.db.WithContext(ctx).Model(new(OrgUser)).Where("uid = ? AND org_id = ?", userID, orgID).Count(&count)
	return count > 0
}

// IsPublicMember returns true if the user with given ID is a public member of
// the organization with given ID.
func (s *OrganizationsStore) IsPublicMember(ctx context.Context, orgID, userID int64) bool {
	var count int64
	s.db.WithContext(ctx).Model(new(OrgUser)).Where("uid = ? AND org_id = ? AND is_public = ?", userID, orgID, true).Count(&count)
	return count > 0
}

// ListOwnedBy returns the organizations that the user with given ID owns,
// most recently updated first.
func (s *OrganizationsStore) ListOwnedBy(ctx context.Context, userID int64) ([]*Organization, error) {
	orgs := make([]*Organization, 0, 10)
	return orgs, s.db.WithContext(ctx).
		Joins(dbx.Quote("JOIN org_user ON org_user.org_id = %s.id", "user")).
		Where("org_user.uid = ? AND org_user.is_owner = ?", userID, true).
		Order(dbx.Quote("%s.updated_unix DESC", "user")).
		Order(dbx.Quote("%s.id DESC", "user")).
		Find(&orgs).Error
}

// listOrgUsers returns at most given number of memberships of the
// organization with given ID, in the order they were created. A limit that is
// not positive means no limit.
func (s *OrganizationsStore) listOrgUsers(ctx context.Context, orgID int64, limit int) ([]*OrgUser, error) {
	db := s.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id ASC")
	if limit > 0 {
		db = db.Limit(limit)
	}
	orgUsers := make([]*OrgUser, 0, 10)
	return orgUsers, db.Find(&orgUsers).Error
}

// ListMembers returns at most given number of members of the organization
// with given ID, in the order they joined. A limit that is not positive means
// no limit.
func (s *OrganizationsStore) ListMembers(ctx context.Context, orgID int64, limit int) ([]*User, error) {
	orgUsers, err := s.listOrgUsers(ctx, orgID, limit)
	if err != nil {
		return nil, errors.Wrap(err, "list organization users")
	}

	members := make([]*User, len(orgUsers))
	for i, ou := range orgUsers {
		members[i], err = newUsersStore(s.db).GetByID(ctx, ou.UID)
		if err != nil {
			return nil, errors.Wrapf(err, "get user %d", ou.UID)
		}
	}
	return members, nil
}

// SetMemberVisibility makes the membership of the user with given ID in the
// organization with given ID public or private. Changing a user who is not a
// member is a no-op.
func (s *OrganizationsStore) SetMemberVisibility(ctx context.Context, orgID, userID int64, public bool) error {
	return s.db.WithContext(ctx).Model(new(OrgUser)).Where("uid = ? AND org_id = ?", userID, orgID).Update("is_public", public).Error
}

// AddMember adds the user with given ID to the organization with given ID.
// Adding a member that the organization has is a no-op.
func (s *OrganizationsStore) AddMember(ctx context.Context, orgID, userID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newOrganizationsStore(tx).addMember(ctx, orgID, userID)
	})
}

// addMember is AddMember without a transaction of its own.
func (s *OrganizationsStore) addMember(ctx context.Context, orgID, userID int64) error {
	if s.IsMember(ctx, orgID, userID) {
		return nil
	}

	db := s.db.WithContext(ctx)
	if err := db.Create(&OrgUser{UID: userID, OrgID: orgID}).Error; err != nil {
		return errors.Wrap(err, "create organization user")
	}
	return db.Model(new(User)).Where("id = ?", orgID).UpdateColumn("num_members", gorm.Expr("num_members + 1")).Error
}

// RemoveMember removes the user with given ID from the organization with given
// ID and its teams, and makes the user stop watching the repositories of the
// organization that the user can read and lose access to them. It returns
// ErrLastOrgOwner when the user is the last owner. Removing a user who is not
// a member is a no-op.
func (s *OrganizationsStore) RemoveMember(ctx context.Context, orgID, userID int64) error {
	db := s.db.WithContext(ctx)
	ou := new(OrgUser)
	err := db.Where("uid = ? AND org_id = ?", userID, orgID).First(ou).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return errors.Wrap(err, "get organization user")
	}

	if ou.IsOwner {
		t, err := newTeamsStore(s.db).GetOwnerTeam(ctx, orgID)
		if err != nil {
			return errors.Wrap(err, "get owner team")
		} else if t.NumMembers == 1 {
			return ErrLastOrgOwner{UID: userID}
		}
	}

	repos, _, err := s.ListAccessibleRepos(ctx, orgID, userID, 1, 0)
	if err != nil {
		return errors.Wrap(err, "list accessible repositories")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Repositories are locked before the organization, the order adding a
		// team member locks them in, so the two cannot deadlock.
		lockIDs := make([]int64, 0, len(repos))
		for _, repo := range repos {
			lockIDs = append(lockIDs, repo.ID)
		}
		if err := lockRepositories(tx, lockIDs); err != nil {
			return err
		}

		if err := tx.Where("id = ?", ou.ID).Delete(new(OrgUser)).Error; err != nil {
			return errors.Wrap(err, "delete organization user")
		}
		err := tx.Model(new(User)).Where("id = ?", orgID).UpdateColumn("num_members", gorm.Expr("num_members - 1")).Error
		if err != nil {
			return errors.Wrap(err, "count members")
		}

		repoIDs := make([]int64, 0, len(repos))
		for _, repo := range repos {
			repoIDs = append(repoIDs, repo.ID)
			if err = newReposStore(tx).unwatch(ctx, userID, repo.ID); err != nil {
				return errors.Wrapf(err, "unwatch repository %d", repo.ID)
			}
		}
		if len(repoIDs) > 0 {
			if err = tx.Where("user_id = ? AND repo_id IN ?", userID, repoIDs).Delete(new(Access)).Error; err != nil {
				return errors.Wrap(err, "delete accesses")
			}
		}

		teams := newTeamsStore(tx)
		memberTeams, err := teams.ListByMember(ctx, orgID, userID)
		if err != nil {
			return errors.Wrap(err, "list teams")
		}
		for _, t := range memberTeams {
			if err = teams.removeMember(ctx, orgID, t.ID, userID); err != nil {
				return errors.Wrapf(err, "remove from team %d", t.ID)
			}
		}
		return nil
	})
}

// teamRepoIDsOfMember returns the IDs of the repositories that the teams of
// the organization with given ID, which the user with given ID is a member
// of, have.
func (s *OrganizationsStore) teamRepoIDsOfMember(ctx context.Context, orgID, userID int64) ([]int64, error) {
	db := s.db.WithContext(ctx)
	var repoIDs []int64
	return repoIDs, db.Model(new(TeamRepo)).
		Where("team_id IN (?)", db.Model(new(TeamUser)).Select("team_id").Where("org_id = ? AND uid = ?", orgID, userID)).
		Distinct().
		Pluck("repo_id", &repoIDs).Error
}

// ListAccessibleRepos returns the repositories of given page of the
// organization with given ID that the user with given ID can see, i.e., those
// that are public and listed, and those of the teams of the user, most
// recently updated first, and the number of them. A page size that is not
// positive means no limit.
func (s *OrganizationsStore) ListAccessibleRepos(ctx context.Context, orgID, userID int64, page, pageSize int) ([]*Repository, int64, error) {
	teamRepoIDs, err := s.teamRepoIDsOfMember(ctx, orgID, userID)
	if err != nil {
		return nil, 0, errors.Wrap(err, "list team repository IDs")
	}

	db := s.db.WithContext(ctx)
	visible := db.Where("is_private = ? AND is_unlisted = ?", false, false)
	if len(teamRepoIDs) > 0 {
		visible = visible.Or("id IN ?", teamRepoIDs)
	}
	query := func() *gorm.DB {
		return db.Model(new(Repository)).Where("owner_id = ?", orgID).Where(visible)
	}

	var count int64
	if err = query().Count(&count).Error; err != nil {
		return nil, 0, errors.Wrap(err, "count repositories")
	}

	list := query().Order("updated_unix DESC").Order("id DESC")
	if pageSize > 0 {
		list = list.Limit(pageSize).Offset((max(page, 1) - 1) * pageSize)
	}
	repos := make([]*Repository, 0, max(pageSize, 0))
	if err = list.Find(&repos).Error; err != nil {
		return nil, 0, errors.Wrap(err, "list repositories")
	}
	return repos, count, nil
}

// ListAccessibleMirrors returns the mirrors of the organization with given ID
// that the user with given ID can see, i.e., those that are public, and those
// of the teams of the user, most recently updated first.
func (s *OrganizationsStore) ListAccessibleMirrors(ctx context.Context, orgID, userID int64) ([]*Repository, error) {
	teamRepoIDs, err := s.teamRepoIDsOfMember(ctx, orgID, userID)
	if err != nil {
		return nil, errors.Wrap(err, "list team repository IDs")
	}

	db := s.db.WithContext(ctx)
	visible := db.Where("is_private = ?", false)
	if len(teamRepoIDs) > 0 {
		visible = visible.Or("id IN ?", teamRepoIDs)
	}
	repos := make([]*Repository, 0, 10)
	return repos, db.Where("owner_id = ? AND is_mirror = ?", orgID, true).
		Where(visible).
		Order("updated_unix DESC").
		Order("id DESC").
		Find(&repos).Error
}

// CountByUser returns the number of organizations the user is a member of.
func (s *OrganizationsStore) CountByUser(ctx context.Context, userID int64) (int64, error) {
	var count int64
	return count, s.db.WithContext(ctx).Model(&OrgUser{}).Where("uid = ?", userID).Count(&count).Error
}

type Organization = User

func (o *Organization) TableName() string {
	return "user"
}

var ErrOrgNotExist = errors.New("Organization does not exist")

// OrgUser represents relations of organizations and their members.
type OrgUser struct {
	ID       int64 `gorm:"primaryKey"`
	UID      int64 `gorm:"column:uid;index:IDX_org_user_uid;uniqueIndex:UQE_org_user_s"`
	OrgID    int64 `gorm:"index:IDX_org_user_org_id;uniqueIndex:UQE_org_user_s"`
	IsPublic bool
	IsOwner  bool
	NumTeams int `gorm:"size:32"`
}
