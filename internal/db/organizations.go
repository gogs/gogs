// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbutil"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/repoutil"
	"gogs.io/gogs/internal/userutil"
)

// OrganizationsStore is the persistent interface for organizations.
type OrganizationsStore interface {
	// Create creates a new organization with the initial owner and persists to
	// database. It returns ErrNameNotAllowed if the given name or pattern of the
	// name is not allowed as an organization name, or ErrOrganizationAlreadyExist
	// when a user or an organization with same name already exists.
	Create(ctx context.Context, name string, ownerID int64, opts CreateOrganizationOptions) (*Organization, error)
	// GetByName returns the organization with given name.
	GetByName(ctx context.Context, name string) (*Organization, error)
	// SearchByName returns a list of organizations whose username or full name
	// matches the given keyword case-insensitively. Results are paginated by given
	// page and page size, and sorted by the given order (e.g. "id DESC"). A total
	// count of all results is also returned. If the order is not given, it's up to
	// the database to decide.
	SearchByName(ctx context.Context, keyword string, page, pageSize int, orderBy string) ([]*Organization, int64, error)
	// List returns a list of organizations filtered by options.
	List(ctx context.Context, opts ListOrganizationsOptions) ([]*Organization, error)
	// CountByUser returns the number of organizations the user is a member of.
	CountByUser(ctx context.Context, userID int64) (int64, error)
	// Count returns the total number of organizations.
	Count(ctx context.Context) int64

	// AddMember adds a new member to the given organization.
	AddMember(ctx context.Context, orgID, userID int64) error
	// RemoveMember removes a member from the given organization.
	RemoveMember(ctx context.Context, orgID, userID int64) error
	// HasMember returns whether the given user is a member of the organization
	// (first), and whether the organization membership is public (second).
	HasMember(ctx context.Context, orgID, userID int64) (bool, bool)
	// ListMembers returns all members of the given organization, and sorted by the
	// given order (e.g. "id ASC").
	ListMembers(ctx context.Context, orgID int64, opts ListOrgMembersOptions) ([]*User, error)
	// IsOwnedBy returns true if the given user is an owner of the organization.
	IsOwnedBy(ctx context.Context, orgID, userID int64) bool
	// SetMemberVisibility sets the visibility of the given user in the organization.
	SetMemberVisibility(ctx context.Context, orgID, userID int64, public bool) error

	// GetTeamByName returns the team with given name under the given organization.
	// It returns ErrTeamNotExist whe not found.
	GetTeamByName(ctx context.Context, orgID int64, name string) (*Team, error)

	// AccessibleRepositoriesByUser returns a range of repositories in the
	// organization that the user has access to and the total number of it. Results
	// are paginated by given page and page size, and sorted by the given order
	// (e.g. "updated_unix DESC").
	AccessibleRepositoriesByUser(ctx context.Context, orgID, userID int64, page, pageSize int, opts AccessibleRepositoriesByUserOptions) ([]*Repository, int64, error)
}

var Organizations OrganizationsStore

var _ OrganizationsStore = (*organizations)(nil)

type organizations struct {
	*gorm.DB
}

// NewOrganizationsStore returns a persistent interface for orgs with given
// database connection.
func NewOrganizationsStore(db *gorm.DB) OrganizationsStore {
	return &organizations{DB: db}
}

func (*organizations) recountMembers(tx *gorm.DB, orgID int64) error {
	/*
		Equivalent SQL for PostgreSQL:

		UPDATE "user"
		SET num_members = (
			SELECT COUNT(*) FROM org_user WHERE org_id = @orgID
		)
		WHERE id = @orgID
	*/
	err := tx.Model(&User{}).
		Where("id = ?", orgID).
		Update(
			"num_members",
			tx.Model(&OrgUser{}).Select("COUNT(*)").Where("org_id = ?", orgID),
		).
		Error
	if err != nil {
		return errors.Wrap(err, `update "user.num_members"`)
	}
	return nil
}

func (db *organizations) AddMember(ctx context.Context, orgID, userID int64) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ou := &OrgUser{
			UserID: userID,
			OrgID:  orgID,
		}
		result := tx.FirstOrCreate(ou, ou)
		if result.Error != nil {
			return errors.Wrap(result.Error, "upsert")
		} else if result.RowsAffected <= 0 {
			return nil // Relation already exists
		}
		return db.recountMembers(tx, orgID)
	})
}

type ErrLastOrgOwner struct {
	args map[string]any
}

func IsErrLastOrgOwner(err error) bool {
	return errors.As(err, &ErrLastOrgOwner{})
}

func (err ErrLastOrgOwner) Error() string {
	return fmt.Sprintf("user is the last owner of the organization: %v", err.args)
}

func (db *organizations) RemoveMember(ctx context.Context, orgID, userID int64) error {
	ou, err := db.getOrgUser(ctx, orgID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // Not a member
		}
		return errors.Wrap(err, "check organization membership")
	}

	// Check if the member to remove is the last owner.
	if ou.IsOwner {
		t, err := db.GetTeamByName(ctx, orgID, TeamNameOwners)
		if err != nil {
			return errors.Wrap(err, "get owners team")
		} else if t.NumMembers == 1 {
			return ErrLastOrgOwner{args: map[string]any{"orgID": orgID, "userID": userID}}
		}
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repoIDsConds := db.accessibleRepositoriesByUser(tx, orgID, userID, accessibleRepositoriesByUserOptions{}).Select("repository.id")

		err := tx.Where("user_id = ? AND repo_id IN (?)", userID, repoIDsConds).Delete(&Watch{}).Error
		if err != nil {
			return errors.Wrap(err, "unwatch repositories")
		}

		err = tx.Table("repository").
			Where("id IN (?)", repoIDsConds).
			UpdateColumn("num_watches", gorm.Expr("num_watches - 1")).
			Error
		if err != nil {
			return errors.Wrap(err, `decrease "repository.num_watches"`)
		}

		err = tx.Where("user_id = ? AND repo_id IN (?)", userID, repoIDsConds).Delete(&Access{}).Error
		if err != nil {
			return errors.Wrap(err, "delete repository accesses")
		}

		err = tx.Where("user_id = ? AND repo_id IN (?)", userID, repoIDsConds).Delete(&Collaboration{}).Error
		if err != nil {
			return errors.Wrap(err, "delete repository collaborations")
		}

		/*
			Equivalent SQL for PostgreSQL:

			UPDATE "team"
			SET num_members = num_members - 1
			WHERE id IN (
				SELECT team_id FROM "team_user"
				WHERE team_user.org_id = @orgID AND uid = @userID)
			)
		*/
		err = tx.Table("team").
			Where(`id IN (?)`, tx.
				Select("team_id").
				Table("team_user").
				Where("org_id = ? AND uid = ?", orgID, userID),
			).
			UpdateColumn("num_members", gorm.Expr("num_members - 1")).
			Error
		if err != nil {
			return errors.Wrap(err, `decrease "team.num_members"`)
		}

		err = tx.Where("uid = ? AND org_id = ?", userID, orgID).Delete(&TeamUser{}).Error
		if err != nil {
			return errors.Wrap(err, "delete team membership")
		}

		err = tx.Where("uid = ? AND org_id = ?", userID, orgID).Delete(&OrgUser{}).Error
		if err != nil {
			return errors.Wrap(err, "delete organization membership")
		}
		return db.recountMembers(tx, orgID)
	})
}

type accessibleRepositoriesByUserOptions struct {
	orderBy  string
	page     int
	pageSize int
}

func (*organizations) accessibleRepositoriesByUser(tx *gorm.DB, orgID, userID int64, opts accessibleRepositoriesByUserOptions) *gorm.DB {
	/*
		Equivalent SQL for PostgreSQL:

		<SELECT * FROM "repository">
		JOIN team_repo ON repository.id = team_repo.repo_id
		WHERE
			owner_id = @orgID
		AND (
				team_repo.team_id IN (
					SELECT team_id FROM "team_user"
					WHERE team_user.org_id = @orgID AND uid = @userID)
				)
			OR  (repository.is_private = FALSE AND repository.is_unlisted = FALSE)
		)
		[ORDER BY updated_unix DESC]
		[LIMIT @limit OFFSET @offset]
	*/
	conds := tx.
		Joins("JOIN team_repo ON repository.id = team_repo.repo_id").
		Where("owner_id = ? AND (?)", orgID, tx.
			Where("team_repo.team_id IN (?)", tx.
				Select("team_id").
				Table("team_user").
				Where("team_user.org_id = ? AND uid = ?", orgID, userID),
			).
			Or("repository.is_private = ? AND repository.is_unlisted = ?", false, false),
		)
	if opts.orderBy != "" {
		conds.Order(opts.orderBy)
	}
	if opts.page > 0 && opts.pageSize > 0 {
		conds.Limit(opts.pageSize).Offset((opts.page - 1) * opts.pageSize)
	}
	return conds
}

type AccessibleRepositoriesByUserOptions struct {
	// Whether to skip counting the total number of repositories.
	SkipCount bool
}

func (db *organizations) AccessibleRepositoriesByUser(ctx context.Context, orgID, userID int64, page, pageSize int, opts AccessibleRepositoriesByUserOptions) ([]*Repository, int64, error) {
	conds := db.accessibleRepositoriesByUser(
		db.DB,
		orgID,
		userID,
		accessibleRepositoriesByUserOptions{
			orderBy:  "updated_unix DESC",
			page:     page,
			pageSize: pageSize,
		},
	).WithContext(ctx)

	repos := make([]*Repository, 0, pageSize)
	err := conds.Find(&repos).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "list repositories")
	}

	if opts.SkipCount {
		return repos, 0, nil
	}
	var count int64
	err = conds.Model(&Repository{}).Count(&count).Error
	if err != nil {
		return nil, 0, errors.Wrap(err, "count repositories")
	}
	return repos, count, nil
}

func (db *organizations) getOrgUser(ctx context.Context, orgID, userID int64) (*OrgUser, error) {
	var ou OrgUser
	return &ou, db.WithContext(ctx).Where("org_id = ? AND uid = ?", orgID, userID).First(&ou).Error
}

func (db *organizations) IsOwnedBy(ctx context.Context, orgID, userID int64) bool {
	ou, err := db.getOrgUser(ctx, orgID, userID)
	return err == nil && ou.IsOwner
}

func (db *organizations) SetMemberVisibility(ctx context.Context, orgID, userID int64, public bool) error {
	return db.Table("org_user").Where("org_id = ? AND uid = ?", orgID, userID).UpdateColumn("is_public", public).Error
}

func (db *organizations) HasMember(ctx context.Context, orgID, userID int64) (bool, bool) {
	ou, err := db.getOrgUser(ctx, orgID, userID)
	return err == nil, ou != nil && ou.IsPublic
}

type ListOrgMembersOptions struct {
	// The maximum number of members to return.
	Limit int
}

func (db *organizations) ListMembers(ctx context.Context, orgID int64, opts ListOrgMembersOptions) ([]*User, error) {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "user"
		JOIN org_user ON org_user.uid = user.id
		WHERE
			org_user.org_id = @orgID
		ORDER BY user.id ASC
		[LIMIT @limit]
	*/
	conds := db.WithContext(ctx).
		Joins(dbutil.Quote("JOIN org_user ON org_user.uid = %s.id", "user")).
		Where("org_user.org_id = ?", orgID).
		Order(dbutil.Quote("%s.id ASC", "user"))
	if opts.Limit > 0 {
		conds.Limit(opts.Limit)
	}
	var users []*User
	return users, conds.Find(&users).Error
}

type ListOrganizationsOptions struct {
	// Filter by the membership with the given user ID.
	MemberID int64
	// Whether to include private memberships.
	IncludePrivateMembers bool
	// 1-based page number.
	Page int
	// Number of results per page.
	PageSize int
}

func (db *organizations) List(ctx context.Context, opts ListOrganizationsOptions) ([]*Organization, error) {
	if opts.MemberID <= 0 {
		return nil, errors.New("MemberID must be greater than 0")
	}

	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "user"
		[JOIN org_user ON org_user.org_id = user.id]
		WHERE
			type = @type
		[AND org_user.uid = @memberID
		AND org_user.is_public = @includePrivateMembers]
		ORDER BY user.id ASC
		[LIMIT @limit OFFSET @offset]
	*/
	conds := db.WithContext(ctx).
		Where("type = ?", UserTypeOrganization).
		Order(dbutil.Quote("%s.id ASC", "user"))

	if opts.MemberID > 0 || !opts.IncludePrivateMembers {
		conds.Joins(dbutil.Quote("JOIN org_user ON org_user.org_id = %s.id", "user"))
	}
	if opts.MemberID > 0 {
		conds.Where("org_user.uid = ?", opts.MemberID)
	}
	if !opts.IncludePrivateMembers {
		conds.Where("org_user.is_public = ?", true)
	}
	if opts.Page > 0 && opts.PageSize > 0 {
		conds.Limit(opts.PageSize).Offset((opts.Page - 1) * opts.PageSize)
	}

	var orgs []*Organization
	return orgs, conds.Find(&orgs).Error
}

type CreateOrganizationOptions struct {
	FullName    string
	Email       string
	Location    string
	Website     string
	Description string
}

type ErrOrganizationAlreadyExist struct {
	args errutil.Args
}

// IsErrOrganizationAlreadyExist returns true if the underlying error has the
// type ErrOrganizationAlreadyExist.
func IsErrOrganizationAlreadyExist(err error) bool {
	return errors.As(err, &ErrOrganizationAlreadyExist{})
}

func (err ErrOrganizationAlreadyExist) Error() string {
	return fmt.Sprintf("organization already exists: %v", err.args)
}

func (db *organizations) Create(ctx context.Context, name string, ownerID int64, opts CreateOrganizationOptions) (*Organization, error) {
	err := isUsernameAllowed(name)
	if err != nil {
		return nil, err
	}

	if NewUsersStore(db.DB).IsUsernameUsed(ctx, name, 0) {
		return nil, ErrOrganizationAlreadyExist{
			args: errutil.Args{
				"name": name,
			},
		}
	}

	org := &Organization{
		LowerName:       strings.ToLower(name),
		Name:            name,
		FullName:        opts.FullName,
		Email:           opts.Email,
		Type:            UserTypeOrganization,
		Location:        opts.Location,
		Website:         opts.Website,
		MaxRepoCreation: -1,
		IsActive:        true,
		UseCustomAvatar: true,
		Description:     opts.Description,
		NumTeams:        1, // The default "owners" team
		NumMembers:      1, // The initial owner
	}

	org.Rands, err = userutil.RandomSalt()
	if err != nil {
		return nil, err
	}
	org.Salt, err = userutil.RandomSalt()
	if err != nil {
		return nil, err
	}

	return org, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Create(org).Error
		if err != nil {
			return errors.Wrap(err, "create organization")
		}

		err = tx.Create(&OrgUser{
			UserID:   ownerID,
			OrgID:    org.ID,
			IsOwner:  true,
			NumTeams: 1,
		}).Error
		if err != nil {
			return errors.Wrap(err, "create org-user relation")
		}

		team := &Team{
			OrgID:      org.ID,
			LowerName:  strings.ToLower(TeamNameOwners),
			Name:       TeamNameOwners,
			Authorize:  AccessModeOwner,
			NumMembers: 1,
		}
		err = tx.Create(team).Error
		if err != nil {
			return errors.Wrap(err, "create owner team")
		}

		err = tx.Create(&TeamUser{
			UID:    ownerID,
			OrgID:  org.ID,
			TeamID: team.ID,
		}).Error
		if err != nil {
			return errors.Wrap(err, "create team-user relation")
		}

		err = userutil.GenerateRandomAvatar(org.ID, org.Name, org.Email)
		if err != nil {
			return errors.Wrap(err, "generate organization avatar")
		}

		err = os.MkdirAll(repoutil.UserPath(org.Name), os.ModePerm)
		if err != nil {
			return errors.Wrap(err, "create organization directory")
		}
		return nil
	})
}

var _ errutil.NotFound = (*ErrUserNotExist)(nil)

type ErrOrganizationNotExist struct {
	args errutil.Args
}

// IsErrOrganizationNotExist returns true if the underlying error has the type
// ErrOrganizationNotExist.
func IsErrOrganizationNotExist(err error) bool {
	return errors.As(err, &ErrOrganizationNotExist{})
}

func (err ErrOrganizationNotExist) Error() string {
	return fmt.Sprintf("organization does not exist: %v", err.args)
}

func (ErrOrganizationNotExist) NotFound() bool {
	return true
}

func (db *organizations) GetByName(ctx context.Context, name string) (*Organization, error) {
	org, err := getUserByUsername(ctx, db.DB, UserTypeOrganization, name)
	if err != nil {
		if IsErrUserNotExist(err) {
			return nil, ErrOrganizationNotExist{args: map[string]any{"name": name}}
		}
		return nil, errors.Wrap(err, "get organization by name")
	}
	return org, nil
}

func (db *organizations) SearchByName(ctx context.Context, keyword string, page, pageSize int, orderBy string) ([]*Organization, int64, error) {
	return searchUserByName(ctx, db.DB, UserTypeOrganization, keyword, page, pageSize, orderBy)
}

func (db *organizations) CountByUser(ctx context.Context, userID int64) (int64, error) {
	var count int64
	return count, db.WithContext(ctx).Model(&OrgUser{}).Where("uid = ?", userID).Count(&count).Error
}

func (db *organizations) Count(ctx context.Context) int64 {
	var count int64
	db.WithContext(ctx).Model(&User{}).Where("type = ?", UserTypeOrganization).Count(&count)
	return count
}

var _ errutil.NotFound = (*ErrTeamNotExist)(nil)

type ErrTeamNotExist struct {
	args map[string]any
}

func IsErrTeamNotExist(err error) bool {
	return errors.As(err, &ErrTeamNotExist{})
}

func (err ErrTeamNotExist) Error() string {
	return fmt.Sprintf("team does not exist: %v", err.args)
}

func (ErrTeamNotExist) NotFound() bool {
	return true
}

func (db *organizations) GetTeamByName(ctx context.Context, orgID int64, name string) (*Team, error) {
	var team Team
	err := db.WithContext(ctx).Where("org_id = ? AND lower_name = ?", orgID, strings.ToLower(name)).First(&team).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTeamNotExist{args: map[string]any{"orgID": orgID, "name": name}}
		}
		return nil, errors.Wrap(err, "get team by name")
	}
	return &team, nil
}

type Organization = User

func (u *Organization) TableName() string {
	return "user"
}

// IsOwnedBy returns true if the given user is an owner of the organization.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.Organization`.
func (u *Organization) IsOwnedBy(userID int64) bool {
	return Organizations.IsOwnedBy(context.TODO(), u.ID, userID)
}

// OrgUser represents relations of organizations and their members.
type OrgUser struct {
	ID       int64 `gorm:"primaryKey"`
	UserID   int64 `xorm:"uid INDEX UNIQUE(s)" gorm:"column:uid;uniqueIndex:org_user_user_org_unique;index;not null" json:"Uid"`
	OrgID    int64 `xorm:"INDEX UNIQUE(s)" gorm:"uniqueIndex:org_user_user_org_unique;index;not null"`
	IsPublic bool  `gorm:"not null;default:FALSE"`
	IsOwner  bool  `gorm:"not null;default:FALSE"`
	NumTeams int   `gorm:"not null;default:0"`
}
