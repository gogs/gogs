// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-macaron/binding"
	api "github.com/gogs/go-gogs-client"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/dbutil"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/strutil"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userutil"
)

// UsersStore is the persistent interface for users.
//
// NOTE: All methods are sorted in alphabetical order.
type UsersStore interface {
	// Authenticate validates username and password via given login source ID. It
	// returns ErrUserNotExist when the user was not found.
	//
	// When the "loginSourceID" is negative, it aborts the process and returns
	// ErrUserNotExist if the user was not found in the database.
	//
	// When the "loginSourceID" is non-negative, it returns ErrLoginSourceMismatch
	// if the user has different login source ID than the "loginSourceID".
	//
	// When the "loginSourceID" is positive, it tries to authenticate via given
	// login source and creates a new user when not yet exists in the database.
	Authenticate(ctx context.Context, username, password string, loginSourceID int64) (*User, error)
	// Count returns the total number of users.
	Count(ctx context.Context) int64
	// Create creates a new user and persists to database. It returns
	// ErrUserAlreadyExist when a user with same name already exists, or
	// ErrEmailAlreadyUsed if the email has been used by another user.
	Create(ctx context.Context, username, email string, opts CreateUserOptions) (*User, error)
	// DeleteCustomAvatar deletes the current user custom avatar and falls back to
	// use look up avatar by email.
	DeleteCustomAvatar(ctx context.Context, userID int64) error
	// GetByEmail returns the user (not organization) with given email. It ignores
	// records with unverified emails and returns ErrUserNotExist when not found.
	GetByEmail(ctx context.Context, email string) (*User, error)
	// GetByID returns the user with given ID. It returns ErrUserNotExist when not
	// found.
	GetByID(ctx context.Context, id int64) (*User, error)
	// GetByUsername returns the user with given username. It returns
	// ErrUserNotExist when not found.
	GetByUsername(ctx context.Context, username string) (*User, error)
	// HasForkedRepository returns true if the user has forked given repository.
	HasForkedRepository(ctx context.Context, userID, repoID int64) bool
	// IsUsernameUsed returns true if the given username has been used.
	IsUsernameUsed(ctx context.Context, username string) bool
	// List returns a list of users. Results are paginated by given page and page
	// size, and sorted by primary key (id) in ascending order.
	List(ctx context.Context, page, pageSize int) ([]*User, error)
	// ListFollowers returns a list of users that are following the given user.
	// Results are paginated by given page and page size, and sorted by the time of
	// follow in descending order.
	ListFollowers(ctx context.Context, userID int64, page, pageSize int) ([]*User, error)
	// ListFollowings returns a list of users that are followed by the given user.
	// Results are paginated by given page and page size, and sorted by the time of
	// follow in descending order.
	ListFollowings(ctx context.Context, userID int64, page, pageSize int) ([]*User, error)
	// UseCustomAvatar uses the given avatar as the user custom avatar.
	UseCustomAvatar(ctx context.Context, userID int64, avatar []byte) error
}

var Users UsersStore

var _ UsersStore = (*users)(nil)

type users struct {
	*gorm.DB
}

// NewUsersStore returns a persistent interface for users with given database
// connection.
func NewUsersStore(db *gorm.DB) UsersStore {
	return &users{DB: db}
}

type ErrLoginSourceMismatch struct {
	args errutil.Args
}

func (err ErrLoginSourceMismatch) Error() string {
	return fmt.Sprintf("login source mismatch: %v", err.args)
}

func (db *users) Authenticate(ctx context.Context, login, password string, loginSourceID int64) (*User, error) {
	login = strings.ToLower(login)

	query := db.WithContext(ctx)
	if strings.Contains(login, "@") {
		query = query.Where("email = ?", login)
	} else {
		query = query.Where("lower_name = ?", login)
	}

	user := new(User)
	err := query.First(user).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, errors.Wrap(err, "get user")
	}

	var authSourceID int64 // The login source ID will be used to authenticate the user
	createNewUser := false // Whether to create a new user after successful authentication

	// User found in the database
	if err == nil {
		// Note: This check is unnecessary but to reduce user confusion at login page
		// and make it more consistent from user's perspective.
		if loginSourceID >= 0 && user.LoginSource != loginSourceID {
			return nil, ErrLoginSourceMismatch{args: errutil.Args{"expect": loginSourceID, "actual": user.LoginSource}}
		}

		// Validate password hash fetched from database for local accounts.
		if user.IsLocal() {
			if userutil.ValidatePassword(user.Password, user.Salt, password) {
				return user, nil
			}

			return nil, auth.ErrBadCredentials{Args: map[string]interface{}{"login": login, "userID": user.ID}}
		}

		authSourceID = user.LoginSource

	} else {
		// Non-local login source is always greater than 0.
		if loginSourceID <= 0 {
			return nil, auth.ErrBadCredentials{Args: map[string]interface{}{"login": login}}
		}

		authSourceID = loginSourceID
		createNewUser = true
	}

	source, err := LoginSources.GetByID(ctx, authSourceID)
	if err != nil {
		return nil, errors.Wrap(err, "get login source")
	}

	if !source.IsActived {
		return nil, errors.Errorf("login source %d is not activated", source.ID)
	}

	extAccount, err := source.Provider.Authenticate(login, password)
	if err != nil {
		return nil, err
	}

	if !createNewUser {
		return user, nil
	}

	// Validate username make sure it satisfies requirement.
	if binding.AlphaDashDotPattern.MatchString(extAccount.Name) {
		return nil, fmt.Errorf("invalid pattern for attribute 'username' [%s]: must be valid alpha or numeric or dash(-_) or dot characters", extAccount.Name)
	}

	return db.Create(ctx, extAccount.Name, extAccount.Email,
		CreateUserOptions{
			FullName:    extAccount.FullName,
			LoginSource: authSourceID,
			LoginName:   extAccount.Login,
			Location:    extAccount.Location,
			Website:     extAccount.Website,
			Activated:   true,
			Admin:       extAccount.Admin,
		},
	)
}

func (db *users) Count(ctx context.Context) int64 {
	var count int64
	db.WithContext(ctx).Model(&User{}).Where("type = ?", UserTypeIndividual).Count(&count)
	return count
}

type CreateUserOptions struct {
	FullName    string
	Password    string
	LoginSource int64
	LoginName   string
	Location    string
	Website     string
	Activated   bool
	Admin       bool
}

type ErrUserAlreadyExist struct {
	args errutil.Args
}

func IsErrUserAlreadyExist(err error) bool {
	_, ok := err.(ErrUserAlreadyExist)
	return ok
}

func (err ErrUserAlreadyExist) Error() string {
	return fmt.Sprintf("user already exists: %v", err.args)
}

type ErrEmailAlreadyUsed struct {
	args errutil.Args
}

func IsErrEmailAlreadyUsed(err error) bool {
	_, ok := err.(ErrEmailAlreadyUsed)
	return ok
}

func (err ErrEmailAlreadyUsed) Email() string {
	email, ok := err.args["email"].(string)
	if ok {
		return email
	}
	return "<email not found>"
}

func (err ErrEmailAlreadyUsed) Error() string {
	return fmt.Sprintf("email has been used: %v", err.args)
}

func (db *users) Create(ctx context.Context, username, email string, opts CreateUserOptions) (*User, error) {
	err := isUsernameAllowed(username)
	if err != nil {
		return nil, err
	}

	if db.IsUsernameUsed(ctx, username) {
		return nil, ErrUserAlreadyExist{
			args: errutil.Args{
				"name": username,
			},
		}
	}

	email = strings.ToLower(email)
	_, err = db.GetByEmail(ctx, email)
	if err == nil {
		return nil, ErrEmailAlreadyUsed{
			args: errutil.Args{
				"email": email,
			},
		}
	} else if !IsErrUserNotExist(err) {
		return nil, err
	}

	user := &User{
		LowerName:       strings.ToLower(username),
		Name:            username,
		FullName:        opts.FullName,
		Email:           email,
		Password:        opts.Password,
		LoginSource:     opts.LoginSource,
		LoginName:       opts.LoginName,
		Location:        opts.Location,
		Website:         opts.Website,
		MaxRepoCreation: -1,
		IsActive:        opts.Activated,
		IsAdmin:         opts.Admin,
		Avatar:          cryptoutil.MD5(email), // Gravatar URL uses the MD5 hash of the email, see https://en.gravatar.com/site/implement/hash/
		AvatarEmail:     email,
	}

	user.Rands, err = userutil.RandomSalt()
	if err != nil {
		return nil, err
	}
	user.Salt, err = userutil.RandomSalt()
	if err != nil {
		return nil, err
	}
	user.Password = userutil.EncodePassword(user.Password, user.Salt)

	return user, db.WithContext(ctx).Create(user).Error
}

func (db *users) DeleteCustomAvatar(ctx context.Context, userID int64) error {
	_ = os.Remove(userutil.CustomAvatarPath(userID))
	return db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"use_custom_avatar": false,
			"updated_unix":      db.NowFunc().Unix(),
		}).
		Error
}

var _ errutil.NotFound = (*ErrUserNotExist)(nil)

type ErrUserNotExist struct {
	args errutil.Args
}

func IsErrUserNotExist(err error) bool {
	_, ok := err.(ErrUserNotExist)
	return ok
}

func (err ErrUserNotExist) Error() string {
	return fmt.Sprintf("user does not exist: %v", err.args)
}

func (ErrUserNotExist) NotFound() bool {
	return true
}

func (db *users) GetByEmail(ctx context.Context, email string) (*User, error) {
	if email == "" {
		return nil, ErrUserNotExist{args: errutil.Args{"email": email}}
	}
	email = strings.ToLower(email)

	// First try to find the user by primary email
	user := new(User)
	err := db.WithContext(ctx).
		Where("email = ? AND type = ? AND is_active = ?", email, UserTypeIndividual, true).
		First(user).
		Error
	if err == nil {
		return user, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Otherwise, check activated email addresses
	emailAddress := new(EmailAddress)
	err = db.WithContext(ctx).
		Where("email = ? AND is_activated = ?", email, true).
		First(emailAddress).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotExist{args: errutil.Args{"email": email}}
		}
		return nil, err
	}

	return db.GetByID(ctx, emailAddress.UserID)
}

func (db *users) GetByID(ctx context.Context, id int64) (*User, error) {
	user := new(User)
	err := db.WithContext(ctx).Where("id = ?", id).First(user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotExist{args: errutil.Args{"userID": id}}
		}
		return nil, err
	}
	return user, nil
}

func (db *users) GetByUsername(ctx context.Context, username string) (*User, error) {
	user := new(User)
	err := db.WithContext(ctx).Where("lower_name = ?", strings.ToLower(username)).First(user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotExist{args: errutil.Args{"name": username}}
		}
		return nil, err
	}
	return user, nil
}

func (db *users) HasForkedRepository(ctx context.Context, userID, repoID int64) bool {
	var count int64
	db.WithContext(ctx).Model(new(Repository)).Where("owner_id = ? AND fork_id = ?", userID, repoID).Count(&count)
	return count > 0
}

func (db *users) IsUsernameUsed(ctx context.Context, username string) bool {
	if username == "" {
		return false
	}
	return db.WithContext(ctx).
		Select("id").
		Where("lower_name = ?", strings.ToLower(username)).
		First(&User{}).
		Error != gorm.ErrRecordNotFound
}

func (db *users) List(ctx context.Context, page, pageSize int) ([]*User, error) {
	users := make([]*User, 0, pageSize)
	return users, db.WithContext(ctx).
		Where("type = ?", UserTypeIndividual).
		Limit(pageSize).Offset((page - 1) * pageSize).
		Order("id ASC").
		Find(&users).
		Error
}

func (db *users) ListFollowers(ctx context.Context, userID int64, page, pageSize int) ([]*User, error) {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "user"
		LEFT JOIN follow ON follow.user_id = "user".id
		WHERE follow.follow_id = @userID
		ORDER BY follow.id DESC
		LIMIT @limit OFFSET @offset
	*/
	users := make([]*User, 0, pageSize)
	return users, db.WithContext(ctx).
		Joins(dbutil.Quote("LEFT JOIN follow ON follow.user_id = %s.id", "user")).
		Where("follow.follow_id = ?", userID).
		Limit(pageSize).Offset((page - 1) * pageSize).
		Order("follow.id DESC").
		Find(&users).
		Error
}

func (db *users) ListFollowings(ctx context.Context, userID int64, page, pageSize int) ([]*User, error) {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "user"
		LEFT JOIN follow ON follow.user_id = "user".id
		WHERE follow.user_id = @userID
		ORDER BY follow.id DESC
		LIMIT @limit OFFSET @offset
	*/
	users := make([]*User, 0, pageSize)
	return users, db.WithContext(ctx).
		Joins(dbutil.Quote("LEFT JOIN follow ON follow.follow_id = %s.id", "user")).
		Where("follow.user_id = ?", userID).
		Limit(pageSize).Offset((page - 1) * pageSize).
		Order("follow.id DESC").
		Find(&users).
		Error
}

func (db *users) UseCustomAvatar(ctx context.Context, userID int64, avatar []byte) error {
	err := userutil.SaveAvatar(userID, avatar)
	if err != nil {
		return errors.Wrap(err, "save avatar")
	}

	return db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"use_custom_avatar": true,
			"updated_unix":      db.NowFunc().Unix(),
		}).
		Error
}

// UserType indicates the type of the user account.
type UserType int

const (
	UserTypeIndividual UserType = iota // NOTE: Historic reason to make it starts at 0.
	UserTypeOrganization
)

// User represents the object of an individual or an organization.
type User struct {
	ID        int64  `gorm:"primaryKey"`
	LowerName string `xorm:"UNIQUE NOT NULL" gorm:"unique;not null"`
	Name      string `xorm:"UNIQUE NOT NULL" gorm:"not null"`
	FullName  string
	// Email is the primary email address (to be used for communication)
	Email       string `xorm:"NOT NULL" gorm:"not null"`
	Password    string `xorm:"passwd NOT NULL" gorm:"column:passwd;not null"`
	LoginSource int64  `xorm:"NOT NULL DEFAULT 0" gorm:"not null;default:0"`
	LoginName   string
	Type        UserType
	Location    string
	Website     string
	Rands       string `xorm:"VARCHAR(10)" gorm:"type:VARCHAR(10)"`
	Salt        string `xorm:"VARCHAR(10)" gorm:"type:VARCHAR(10)"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix int64

	// Remember visibility choice for convenience, true for private
	LastRepoVisibility bool
	// Maximum repository creation limit, -1 means use global default
	MaxRepoCreation int `xorm:"NOT NULL DEFAULT -1" gorm:"not null;default:-1"`

	// Permissions
	IsActive         bool // Activate primary email
	IsAdmin          bool
	AllowGitHook     bool
	AllowImportLocal bool // Allow migrate repository by local path
	ProhibitLogin    bool

	// Avatar
	Avatar          string `xorm:"VARCHAR(2048) NOT NULL" gorm:"type:VARCHAR(2048);not null"`
	AvatarEmail     string `xorm:"NOT NULL" gorm:"not null"`
	UseCustomAvatar bool

	// Counters
	NumFollowers int
	NumFollowing int `xorm:"NOT NULL DEFAULT 0" gorm:"not null;default:0"`
	NumStars     int
	NumRepos     int

	// For organization
	Description string
	NumTeams    int
	NumMembers  int
	Teams       []*Team `xorm:"-" gorm:"-" json:"-"`
	Members     []*User `xorm:"-" gorm:"-" json:"-"`
}

// BeforeCreate implements the GORM create hook.
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.CreatedUnix == 0 {
		u.CreatedUnix = tx.NowFunc().Unix()
		u.UpdatedUnix = u.CreatedUnix
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (u *User) AfterFind(_ *gorm.DB) error {
	u.Created = time.Unix(u.CreatedUnix, 0).Local()
	u.Updated = time.Unix(u.UpdatedUnix, 0).Local()
	return nil
}

// IsLocal returns true if the user is created as local account.
func (u *User) IsLocal() bool {
	return u.LoginSource <= 0
}

// IsOrganization returns true if the user is an organization.
func (u *User) IsOrganization() bool {
	return u.Type == UserTypeOrganization
}

// IsMailable returns true if the user is eligible to receive emails.
func (u *User) IsMailable() bool {
	return u.IsActive
}

// APIFormat returns the API format of a user.
func (u *User) APIFormat() *api.User {
	return &api.User{
		ID:        u.ID,
		UserName:  u.Name,
		Login:     u.Name,
		FullName:  u.FullName,
		Email:     u.Email,
		AvatarUrl: u.AvatarURL(),
	}
}

// maxNumRepos returns the maximum number of repositories that the user can have
// direct ownership.
func (u *User) maxNumRepos() int {
	if u.MaxRepoCreation <= -1 {
		return conf.Repository.MaxCreationLimit
	}
	return u.MaxRepoCreation
}

// canCreateRepo returns true if the user can create a repository.
func (u *User) canCreateRepo() bool {
	return u.maxNumRepos() <= -1 || u.NumRepos < u.maxNumRepos()
}

// CanCreateOrganization returns true if user can create organizations.
func (u *User) CanCreateOrganization() bool {
	return !conf.Admin.DisableRegularOrgCreation || u.IsAdmin
}

// CanEditGitHook returns true if user can edit Git hooks.
func (u *User) CanEditGitHook() bool {
	return u.IsAdmin || u.AllowGitHook
}

// CanImportLocal returns true if user can migrate repositories by local path.
func (u *User) CanImportLocal() bool {
	return conf.Repository.EnableLocalPathMigration && (u.IsAdmin || u.AllowImportLocal)
}

// DisplayName returns the full name of the user if it's not empty, returns the
// username otherwise.
func (u *User) DisplayName() string {
	if len(u.FullName) > 0 {
		return u.FullName
	}
	return u.Name
}

// HomeURLPath returns the URL path to the user or organization home page.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User` and move this to the "userutil"
// package.
func (u *User) HomeURLPath() string {
	return conf.Server.Subpath + "/" + u.Name
}

// HTMLURL returns the full URL to the user or organization home page.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User` and move this to the "userutil"
// package.
func (u *User) HTMLURL() string {
	return conf.Server.ExternalURL + u.Name
}

// AvatarURLPath returns the URL path to the user or organization avatar. If the
// user enables Gravatar-like service, then an external URL will be returned.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User` and move this to the "userutil"
// package.
func (u *User) AvatarURLPath() string {
	defaultURLPath := conf.UserDefaultAvatarURLPath()
	if u.ID <= 0 {
		return defaultURLPath
	}

	hasCustomAvatar := osutil.IsFile(userutil.CustomAvatarPath(u.ID))
	switch {
	case u.UseCustomAvatar:
		if !hasCustomAvatar {
			return defaultURLPath
		}
		return fmt.Sprintf("%s/%s/%d", conf.Server.Subpath, conf.UsersAvatarPathPrefix, u.ID)
	case conf.Picture.DisableGravatar:
		if !hasCustomAvatar {
			if err := userutil.GenerateRandomAvatar(u.ID, u.Name, u.Email); err != nil {
				log.Error("Failed to generate random avatar [user_id: %d]: %v", u.ID, err)
			}
		}
		return fmt.Sprintf("%s/%s/%d", conf.Server.Subpath, conf.UsersAvatarPathPrefix, u.ID)
	}
	return tool.AvatarLink(u.AvatarEmail)
}

// AvatarURL returns the full URL to the user or organization avatar. If the
// user enables Gravatar-like service, then an external URL will be returned.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User` and move this to the "userutil"
// package.
func (u *User) AvatarURL() string {
	link := u.AvatarURLPath()
	if link[0] == '/' && link[1] != '/' {
		return conf.Server.ExternalURL + strings.TrimPrefix(link, conf.Server.Subpath)[1:]
	}
	return link
}

// IsFollowing returns true if the user is following the given user.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User`.
func (u *User) IsFollowing(followID int64) bool {
	return Follows.IsFollowing(context.TODO(), u.ID, followID)
}

// IsUserOrgOwner returns true if the user is in the owner team of the given
// organization.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User`.
func (u *User) IsUserOrgOwner(orgId int64) bool {
	return IsOrganizationOwner(orgId, u.ID)
}

// IsPublicMember returns true if the user has public membership of the given
// organization.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User`.
func (u *User) IsPublicMember(orgId int64) bool {
	return IsPublicMembership(orgId, u.ID)
}

// GetOrganizationCount returns the count of organization membership that the
// user has.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User`.
func (u *User) GetOrganizationCount() (int64, error) {
	return OrgUsers.CountByUser(context.TODO(), u.ID)
}

// ShortName truncates and returns the username at most in given length.
//
// TODO(unknwon): This is also used in templates, which should be fixed by
// having a dedicated type `template.User`.
func (u *User) ShortName(length int) string {
	return strutil.Ellipsis(u.Name, length)
}

// NewGhostUser creates and returns a fake user for people who has deleted their
// accounts.
//
// TODO: Once migrated to unknwon.dev/i18n, pass in the `i18n.Locale` to
// translate the text to local language.
func NewGhostUser() *User {
	return &User{
		ID:        -1,
		Name:      "Ghost",
		LowerName: "ghost",
	}
}

var (
	reservedUsernames = map[string]struct{}{
		"-":        {},
		"explore":  {},
		"create":   {},
		"assets":   {},
		"css":      {},
		"img":      {},
		"js":       {},
		"less":     {},
		"plugins":  {},
		"debug":    {},
		"raw":      {},
		"install":  {},
		"api":      {},
		"avatar":   {},
		"user":     {},
		"org":      {},
		"help":     {},
		"stars":    {},
		"issues":   {},
		"pulls":    {},
		"commits":  {},
		"repo":     {},
		"template": {},
		"admin":    {},
		"new":      {},
		".":        {},
		"..":       {},
	}
	reservedUsernamePatterns = []string{"*.keys"}
)

type ErrNameNotAllowed struct {
	args errutil.Args
}

func IsErrNameNotAllowed(err error) bool {
	_, ok := err.(ErrNameNotAllowed)
	return ok
}

func (err ErrNameNotAllowed) Value() string {
	val, ok := err.args["name"].(string)
	if ok {
		return val
	}

	val, ok = err.args["pattern"].(string)
	if ok {
		return val
	}

	return "<value not found>"
}

func (err ErrNameNotAllowed) Error() string {
	return fmt.Sprintf("name is not allowed: %v", err.args)
}

// isNameAllowed checks if the name is reserved or pattern of the name is not
// allowed based on given reserved names and patterns. Names are exact match,
// patterns can be prefix or suffix match with the wildcard ("*").
func isNameAllowed(names map[string]struct{}, patterns []string, name string) error {
	name = strings.TrimSpace(strings.ToLower(name))
	if utf8.RuneCountInString(name) == 0 {
		return ErrNameNotAllowed{
			args: errutil.Args{
				"reason": "empty name",
			},
		}
	}

	if _, ok := names[name]; ok {
		return ErrNameNotAllowed{
			args: errutil.Args{
				"reason": "reserved",
				"name":   name,
			},
		}
	}

	for _, pattern := range patterns {
		if pattern[0] == '*' && strings.HasSuffix(name, pattern[1:]) ||
			(pattern[len(pattern)-1] == '*' && strings.HasPrefix(name, pattern[:len(pattern)-1])) {
			return ErrNameNotAllowed{
				args: errutil.Args{
					"reason":  "reserved",
					"pattern": pattern,
				},
			}
		}
	}

	return nil
}

// isUsernameAllowed returns ErrNameNotAllowed if the given name or pattern of
// the name is not allowed as a username.
func isUsernameAllowed(name string) error {
	return isNameAllowed(reservedUsernames, reservedUsernamePatterns, name)
}
