// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nfnt/resize"
	"github.com/unknwon/com"
	"golang.org/x/crypto/pbkdf2"
	log "unknwon.dev/clog/v2"
	"xorm.io/xorm"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/strutil"
	"gogs.io/gogs/internal/tool"
)

// USER_AVATAR_URL_PREFIX is used to identify a URL is to access user avatar.
const USER_AVATAR_URL_PREFIX = "avatars"

type UserType int

const (
	UserIndividual UserType = iota // Historic reason to make it starts at 0.
	UserOrganization
)

// User represents the object of individual and member of organization.
type User struct {
	ID        int64
	LowerName string `xorm:"UNIQUE NOT NULL" gorm:"UNIQUE"`
	Name      string `xorm:"UNIQUE NOT NULL" gorm:"NOT NULL"`
	FullName  string
	// Email is the primary email address (to be used for communication)
	Email       string `xorm:"NOT NULL" gorm:"NOT NULL"`
	Passwd      string `xorm:"NOT NULL" gorm:"NOT NULL"`
	LoginSource int64  `xorm:"NOT NULL DEFAULT 0" gorm:"NOT NULL;DEFAULT:0"`
	LoginName   string
	Type        UserType
	OwnedOrgs   []*User       `xorm:"-" gorm:"-" json:"-"`
	Orgs        []*User       `xorm:"-" gorm:"-" json:"-"`
	Repos       []*Repository `xorm:"-" gorm:"-" json:"-"`
	Location    string
	Website     string
	Rands       string `xorm:"VARCHAR(10)" gorm:"TYPE:VARCHAR(10)"`
	Salt        string `xorm:"VARCHAR(10)" gorm:"TYPE:VARCHAR(10)"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix int64

	// Remember visibility choice for convenience, true for private
	LastRepoVisibility bool
	// Maximum repository creation limit, -1 means use global default
	MaxRepoCreation int `xorm:"NOT NULL DEFAULT -1" gorm:"NOT NULL;DEFAULT:-1"`

	// Permissions
	IsActive         bool // Activate primary email
	IsAdmin          bool
	AllowGitHook     bool
	AllowImportLocal bool // Allow migrate repository by local path
	ProhibitLogin    bool

	// Avatar
	Avatar          string `xorm:"VARCHAR(2048) NOT NULL" gorm:"TYPE:VARCHAR(2048);NOT NULL"`
	AvatarEmail     string `xorm:"NOT NULL" gorm:"NOT NULL"`
	UseCustomAvatar bool

	// Counters
	NumFollowers int
	NumFollowing int `xorm:"NOT NULL DEFAULT 0" gorm:"NOT NULL;DEFAULT:0"`
	NumStars     int
	NumRepos     int

	// For organization
	Description string
	NumTeams    int
	NumMembers  int
	Teams       []*Team `xorm:"-" gorm:"-" json:"-"`
	Members     []*User `xorm:"-" gorm:"-" json:"-"`
}

func (u *User) BeforeInsert() {
	u.CreatedUnix = time.Now().Unix()
	u.UpdatedUnix = u.CreatedUnix
}

func (u *User) BeforeUpdate() {
	if u.MaxRepoCreation < -1 {
		u.MaxRepoCreation = -1
	}
	u.UpdatedUnix = time.Now().Unix()
}

func (u *User) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		u.Created = time.Unix(u.CreatedUnix, 0).Local()
	case "updated_unix":
		u.Updated = time.Unix(u.UpdatedUnix, 0).Local()
	}
}

// IDStr returns string representation of user's ID.
func (u *User) IDStr() string {
	return com.ToStr(u.ID)
}

func (u *User) APIFormat() *api.User {
	return &api.User{
		ID:        u.ID,
		UserName:  u.Name,
		Login:     u.Name,
		FullName:  u.FullName,
		Email:     u.Email,
		AvatarUrl: u.AvatarLink(),
	}
}

// returns true if user login type is LoginPlain.
func (u *User) IsLocal() bool {
	return u.LoginSource <= 0
}

// HasForkedRepo checks if user has already forked a repository with given ID.
func (u *User) HasForkedRepo(repoID int64) bool {
	_, has, _ := HasForkedRepo(u.ID, repoID)
	return has
}

func (u *User) RepoCreationNum() int {
	if u.MaxRepoCreation <= -1 {
		return conf.Repository.MaxCreationLimit
	}
	return u.MaxRepoCreation
}

func (u *User) CanCreateRepo() bool {
	if u.MaxRepoCreation <= -1 {
		if conf.Repository.MaxCreationLimit <= -1 {
			return true
		}
		return u.NumRepos < conf.Repository.MaxCreationLimit
	}
	return u.NumRepos < u.MaxRepoCreation
}

func (u *User) CanCreateOrganization() bool {
	return !conf.Admin.DisableRegularOrgCreation || u.IsAdmin
}

// CanEditGitHook returns true if user can edit Git hooks.
func (u *User) CanEditGitHook() bool {
	return u.IsAdmin || u.AllowGitHook
}

// CanImportLocal returns true if user can migrate repository by local path.
func (u *User) CanImportLocal() bool {
	return conf.Repository.EnableLocalPathMigration && (u.IsAdmin || u.AllowImportLocal)
}

// DashboardLink returns the user dashboard page link.
func (u *User) DashboardLink() string {
	if u.IsOrganization() {
		return conf.Server.Subpath + "/org/" + u.Name + "/dashboard/"
	}
	return conf.Server.Subpath + "/"
}

// HomeLink returns the user or organization home page link.
func (u *User) HomeLink() string {
	return conf.Server.Subpath + "/" + u.Name
}

func (u *User) HTMLURL() string {
	return conf.Server.ExternalURL + u.Name
}

// GenerateEmailActivateCode generates an activate code based on user information and given e-mail.
func (u *User) GenerateEmailActivateCode(email string) string {
	code := tool.CreateTimeLimitCode(
		com.ToStr(u.ID)+email+u.LowerName+u.Passwd+u.Rands,
		conf.Auth.ActivateCodeLives, nil)

	// Add tail hex username
	code += hex.EncodeToString([]byte(u.LowerName))
	return code
}

// GenerateActivateCode generates an activate code based on user information.
func (u *User) GenerateActivateCode() string {
	return u.GenerateEmailActivateCode(u.Email)
}

// CustomAvatarPath returns user custom avatar file path.
func (u *User) CustomAvatarPath() string {
	return filepath.Join(conf.Picture.AvatarUploadPath, com.ToStr(u.ID))
}

// GenerateRandomAvatar generates a random avatar for user.
func (u *User) GenerateRandomAvatar() error {
	seed := u.Email
	if len(seed) == 0 {
		seed = u.Name
	}

	img, err := avatar.RandomImage([]byte(seed))
	if err != nil {
		return fmt.Errorf("RandomImage: %v", err)
	}
	if err = os.MkdirAll(filepath.Dir(u.CustomAvatarPath()), os.ModePerm); err != nil {
		return fmt.Errorf("MkdirAll: %v", err)
	}
	fw, err := os.Create(u.CustomAvatarPath())
	if err != nil {
		return fmt.Errorf("Create: %v", err)
	}
	defer fw.Close()

	if err = png.Encode(fw, img); err != nil {
		return fmt.Errorf("Encode: %v", err)
	}

	log.Info("New random avatar created: %d", u.ID)
	return nil
}

// RelAvatarLink returns relative avatar link to the site domain,
// which includes app sub-url as prefix. However, it is possible
// to return full URL if user enables Gravatar-like service.
func (u *User) RelAvatarLink() string {
	defaultImgUrl := conf.Server.Subpath + "/img/avatar_default.png"
	if u.ID == -1 {
		return defaultImgUrl
	}

	switch {
	case u.UseCustomAvatar:
		if !com.IsExist(u.CustomAvatarPath()) {
			return defaultImgUrl
		}
		return fmt.Sprintf("%s/%s/%d", conf.Server.Subpath, USER_AVATAR_URL_PREFIX, u.ID)
	case conf.Picture.DisableGravatar:
		if !com.IsExist(u.CustomAvatarPath()) {
			if err := u.GenerateRandomAvatar(); err != nil {
				log.Error("GenerateRandomAvatar: %v", err)
			}
		}

		return fmt.Sprintf("%s/%s/%d", conf.Server.Subpath, USER_AVATAR_URL_PREFIX, u.ID)
	}
	return tool.AvatarLink(u.AvatarEmail)
}

// AvatarLink returns user avatar absolute link.
func (u *User) AvatarLink() string {
	link := u.RelAvatarLink()
	if link[0] == '/' && link[1] != '/' {
		return conf.Server.ExternalURL + strings.TrimPrefix(link, conf.Server.Subpath)[1:]
	}
	return link
}

// User.GetFollowers returns range of user's followers.
func (u *User) GetFollowers(page int) ([]*User, error) {
	users := make([]*User, 0, ItemsPerPage)
	sess := x.Limit(ItemsPerPage, (page-1)*ItemsPerPage).Where("follow.follow_id=?", u.ID)
	if conf.UsePostgreSQL {
		sess = sess.Join("LEFT", "follow", `"user".id=follow.user_id`)
	} else {
		sess = sess.Join("LEFT", "follow", "user.id=follow.user_id")
	}
	return users, sess.Find(&users)
}

func (u *User) IsFollowing(followID int64) bool {
	return IsFollowing(u.ID, followID)
}

// GetFollowing returns range of user's following.
func (u *User) GetFollowing(page int) ([]*User, error) {
	users := make([]*User, 0, ItemsPerPage)
	sess := x.Limit(ItemsPerPage, (page-1)*ItemsPerPage).Where("follow.user_id=?", u.ID)
	if conf.UsePostgreSQL {
		sess = sess.Join("LEFT", "follow", `"user".id=follow.follow_id`)
	} else {
		sess = sess.Join("LEFT", "follow", "user.id=follow.follow_id")
	}
	return users, sess.Find(&users)
}

// NewGitSig generates and returns the signature of given user.
func (u *User) NewGitSig() *git.Signature {
	return &git.Signature{
		Name:  u.DisplayName(),
		Email: u.Email,
		When:  time.Now(),
	}
}

// EncodePassword encodes password to safe format.
func (u *User) EncodePassword() {
	newPasswd := pbkdf2.Key([]byte(u.Passwd), []byte(u.Salt), 10000, 50, sha256.New)
	u.Passwd = fmt.Sprintf("%x", newPasswd)
}

// ValidatePassword checks if given password matches the one belongs to the user.
func (u *User) ValidatePassword(passwd string) bool {
	newUser := &User{Passwd: passwd, Salt: u.Salt}
	newUser.EncodePassword()
	return subtle.ConstantTimeCompare([]byte(u.Passwd), []byte(newUser.Passwd)) == 1
}

// UploadAvatar saves custom avatar for user.
// FIXME: split uploads to different subdirs in case we have massive number of users.
func (u *User) UploadAvatar(data []byte) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode image: %v", err)
	}

	_ = os.MkdirAll(conf.Picture.AvatarUploadPath, os.ModePerm)
	fw, err := os.Create(u.CustomAvatarPath())
	if err != nil {
		return fmt.Errorf("create custom avatar directory: %v", err)
	}
	defer fw.Close()

	m := resize.Resize(avatar.AVATAR_SIZE, avatar.AVATAR_SIZE, img, resize.NearestNeighbor)
	if err = png.Encode(fw, m); err != nil {
		return fmt.Errorf("encode image: %v", err)
	}

	return nil
}

// DeleteAvatar deletes the user's custom avatar.
func (u *User) DeleteAvatar() error {
	log.Trace("DeleteAvatar [%d]: %s", u.ID, u.CustomAvatarPath())
	if err := os.Remove(u.CustomAvatarPath()); err != nil {
		return err
	}

	u.UseCustomAvatar = false
	return UpdateUser(u)
}

// IsAdminOfRepo returns true if user has admin or higher access of repository.
func (u *User) IsAdminOfRepo(repo *Repository) bool {
	return Perms.Authorize(u.ID, repo.ID, AccessModeAdmin,
		AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		},
	)
}

// IsWriterOfRepo returns true if user has write access to given repository.
func (u *User) IsWriterOfRepo(repo *Repository) bool {
	return Perms.Authorize(u.ID, repo.ID, AccessModeWrite,
		AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		},
	)
}

// IsOrganization returns true if user is actually a organization.
func (u *User) IsOrganization() bool {
	return u.Type == UserOrganization
}

// IsUserOrgOwner returns true if user is in the owner team of given organization.
func (u *User) IsUserOrgOwner(orgId int64) bool {
	return IsOrganizationOwner(orgId, u.ID)
}

// IsPublicMember returns true if user public his/her membership in give organization.
func (u *User) IsPublicMember(orgId int64) bool {
	return IsPublicMembership(orgId, u.ID)
}

// IsEnabledTwoFactor returns true if user has enabled two-factor authentication.
func (u *User) IsEnabledTwoFactor() bool {
	return TwoFactors.IsUserEnabled(u.ID)
}

func (u *User) getOrganizationCount(e Engine) (int64, error) {
	return e.Where("uid=?", u.ID).Count(new(OrgUser))
}

// GetOrganizationCount returns count of membership of organization of user.
func (u *User) GetOrganizationCount() (int64, error) {
	return u.getOrganizationCount(x)
}

// GetRepositories returns repositories that user owns, including private repositories.
func (u *User) GetRepositories(page, pageSize int) (err error) {
	u.Repos, err = GetUserRepositories(&UserRepoOptions{
		UserID:   u.ID,
		Private:  true,
		Page:     page,
		PageSize: pageSize,
	})
	return err
}

// GetRepositories returns mirror repositories that user owns, including private repositories.
func (u *User) GetMirrorRepositories() ([]*Repository, error) {
	return GetUserMirrorRepositories(u.ID)
}

// GetOwnedOrganizations returns all organizations that user owns.
func (u *User) GetOwnedOrganizations() (err error) {
	u.OwnedOrgs, err = GetOwnedOrgsByUserID(u.ID)
	return err
}

// GetOrganizations returns all organizations that user belongs to.
func (u *User) GetOrganizations(showPrivate bool) error {
	orgIDs, err := GetOrgIDsByUserID(u.ID, showPrivate)
	if err != nil {
		return fmt.Errorf("GetOrgIDsByUserID: %v", err)
	}
	if len(orgIDs) == 0 {
		return nil
	}

	u.Orgs = make([]*User, 0, len(orgIDs))
	if err = x.Where("type = ?", UserOrganization).In("id", orgIDs).Find(&u.Orgs); err != nil {
		return err
	}
	return nil
}

// DisplayName returns full name if it's not empty,
// returns username otherwise.
func (u *User) DisplayName() string {
	if len(u.FullName) > 0 {
		return u.FullName
	}
	return u.Name
}

func (u *User) ShortName(length int) string {
	return tool.EllipsisString(u.Name, length)
}

// IsMailable checks if a user is eligible
// to receive emails.
func (u *User) IsMailable() bool {
	return u.IsActive
}

// IsUserExist checks if given user name exist,
// the user name should be noncased unique.
// If uid is presented, then check will rule out that one,
// it is used when update a user name in settings page.
func IsUserExist(uid int64, name string) (bool, error) {
	if len(name) == 0 {
		return false, nil
	}
	return x.Where("id != ?", uid).Get(&User{LowerName: strings.ToLower(name)})
}

// GetUserSalt returns a random user salt token.
func GetUserSalt() (string, error) {
	return strutil.RandomChars(10)
}

// NewGhostUser creates and returns a fake user for someone who has deleted his/her account.
func NewGhostUser() *User {
	return &User{
		ID:        -1,
		Name:      "Ghost",
		LowerName: "ghost",
	}
}

var (
	reservedUsernames    = []string{"-", "explore", "create", "assets", "css", "img", "js", "less", "plugins", "debug", "raw", "install", "api", "avatar", "user", "org", "help", "stars", "issues", "pulls", "commits", "repo", "template", "admin", "new", ".", ".."}
	reservedUserPatterns = []string{"*.keys"}
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

// isNameAllowed checks if name is reserved or pattern of name is not allowed
// based on given reserved names and patterns.
// Names are exact match, patterns can be prefix or suffix match with placeholder '*'.
func isNameAllowed(names, patterns []string, name string) error {
	name = strings.TrimSpace(strings.ToLower(name))
	if utf8.RuneCountInString(name) == 0 {
		return ErrNameNotAllowed{args: errutil.Args{"reason": "empty name"}}
	}

	for i := range names {
		if name == names[i] {
			return ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "name": name}}
		}
	}

	for _, pat := range patterns {
		if pat[0] == '*' && strings.HasSuffix(name, pat[1:]) ||
			(pat[len(pat)-1] == '*' && strings.HasPrefix(name, pat[:len(pat)-1])) {
			return ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "pattern": pat}}
		}
	}

	return nil
}

// isUsernameAllowed return an error if given name is a reserved name or pattern for users.
func isUsernameAllowed(name string) error {
	return isNameAllowed(reservedUsernames, reservedUserPatterns, name)
}

// CreateUser creates record of a new user.
// Deprecated: Use Users.Create instead.
func CreateUser(u *User) (err error) {
	if err = isUsernameAllowed(u.Name); err != nil {
		return err
	}

	isExist, err := IsUserExist(0, u.Name)
	if err != nil {
		return err
	} else if isExist {
		return ErrUserAlreadyExist{args: errutil.Args{"name": u.Name}}
	}

	u.Email = strings.ToLower(u.Email)
	isExist, err = IsEmailUsed(u.Email)
	if err != nil {
		return err
	} else if isExist {
		return ErrEmailAlreadyUsed{args: errutil.Args{"email": u.Email}}
	}

	u.LowerName = strings.ToLower(u.Name)
	u.AvatarEmail = u.Email
	u.Avatar = tool.HashEmail(u.AvatarEmail)
	if u.Rands, err = GetUserSalt(); err != nil {
		return err
	}
	if u.Salt, err = GetUserSalt(); err != nil {
		return err
	}
	u.EncodePassword()
	u.MaxRepoCreation = -1

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(u); err != nil {
		return err
	} else if err = os.MkdirAll(UserPath(u.Name), os.ModePerm); err != nil {
		return err
	}

	return sess.Commit()
}

func countUsers(e Engine) int64 {
	count, _ := e.Where("type=0").Count(new(User))
	return count
}

// CountUsers returns number of users.
func CountUsers() int64 {
	return countUsers(x)
}

// Users returns number of users in given page.
func ListUsers(page, pageSize int) ([]*User, error) {
	users := make([]*User, 0, pageSize)
	return users, x.Limit(pageSize, (page-1)*pageSize).Where("type=0").Asc("id").Find(&users)
}

// parseUserFromCode returns user by username encoded in code.
// It returns nil if code or username is invalid.
func parseUserFromCode(code string) (user *User) {
	if len(code) <= tool.TIME_LIMIT_CODE_LENGTH {
		return nil
	}

	// Use tail hex username to query user
	hexStr := code[tool.TIME_LIMIT_CODE_LENGTH:]
	if b, err := hex.DecodeString(hexStr); err == nil {
		if user, err = GetUserByName(string(b)); user != nil {
			return user
		} else if !IsErrUserNotExist(err) {
			log.Error("Failed to get user by name %q: %v", string(b), err)
		}
	}

	return nil
}

// verify active code when active account
func VerifyUserActiveCode(code string) (user *User) {
	minutes := conf.Auth.ActivateCodeLives

	if user = parseUserFromCode(code); user != nil {
		// time limit code
		prefix := code[:tool.TIME_LIMIT_CODE_LENGTH]
		data := com.ToStr(user.ID) + user.Email + user.LowerName + user.Passwd + user.Rands

		if tool.VerifyTimeLimitCode(data, minutes, prefix) {
			return user
		}
	}
	return nil
}

// verify active code when active account
func VerifyActiveEmailCode(code, email string) *EmailAddress {
	minutes := conf.Auth.ActivateCodeLives

	if user := parseUserFromCode(code); user != nil {
		// time limit code
		prefix := code[:tool.TIME_LIMIT_CODE_LENGTH]
		data := com.ToStr(user.ID) + email + user.LowerName + user.Passwd + user.Rands

		if tool.VerifyTimeLimitCode(data, minutes, prefix) {
			emailAddress := &EmailAddress{Email: email}
			if has, _ := x.Get(emailAddress); has {
				return emailAddress
			}
		}
	}
	return nil
}

// ChangeUserName changes all corresponding setting from old user name to new one.
func ChangeUserName(u *User, newUserName string) (err error) {
	if err = isUsernameAllowed(newUserName); err != nil {
		return err
	}

	isExist, err := IsUserExist(0, newUserName)
	if err != nil {
		return err
	} else if isExist {
		return ErrUserAlreadyExist{args: errutil.Args{"name": newUserName}}
	}

	if err = ChangeUsernameInPullRequests(u.Name, newUserName); err != nil {
		return fmt.Errorf("ChangeUsernameInPullRequests: %v", err)
	}

	// Delete all local copies of repositories and wikis the user owns.
	if err = x.Where("owner_id=?", u.ID).Iterate(new(Repository), func(idx int, bean interface{}) error {
		repo := bean.(*Repository)
		deleteRepoLocalCopy(repo)
		// TODO: By the same reasoning, shouldn't we also sync access to the local wiki path?
		RemoveAllWithNotice("Delete repository wiki local copy", repo.LocalWikiPath())
		return nil
	}); err != nil {
		return fmt.Errorf("delete repository and wiki local copy: %v", err)
	}

	// Rename or create user base directory
	baseDir := UserPath(u.Name)
	newBaseDir := UserPath(newUserName)
	if com.IsExist(baseDir) {
		return os.Rename(baseDir, newBaseDir)
	}
	return os.MkdirAll(newBaseDir, os.ModePerm)
}

func updateUser(e Engine, u *User) error {
	// Organization does not need email
	if !u.IsOrganization() {
		u.Email = strings.ToLower(u.Email)
		has, err := e.Where("id!=?", u.ID).And("type=?", u.Type).And("email=?", u.Email).Get(new(User))
		if err != nil {
			return err
		} else if has {
			return ErrEmailAlreadyUsed{args: errutil.Args{"email": u.Email}}
		}

		if len(u.AvatarEmail) == 0 {
			u.AvatarEmail = u.Email
		}
		u.Avatar = tool.HashEmail(u.AvatarEmail)
	}

	u.LowerName = strings.ToLower(u.Name)
	u.Location = tool.TruncateString(u.Location, 255)
	u.Website = tool.TruncateString(u.Website, 255)
	u.Description = tool.TruncateString(u.Description, 255)

	_, err := e.ID(u.ID).AllCols().Update(u)
	return err
}

// UpdateUser updates user's information.
func UpdateUser(u *User) error {
	return updateUser(x, u)
}

// deleteBeans deletes all given beans, beans should contain delete conditions.
func deleteBeans(e Engine, beans ...interface{}) (err error) {
	for i := range beans {
		if _, err = e.Delete(beans[i]); err != nil {
			return err
		}
	}
	return nil
}

// FIXME: need some kind of mechanism to record failure. HINT: system notice
func deleteUser(e *xorm.Session, u *User) error {
	// Note: A user owns any repository or belongs to any organization
	//	cannot perform delete operation.

	// Check ownership of repository.
	count, err := getRepositoryCount(e, u)
	if err != nil {
		return fmt.Errorf("GetRepositoryCount: %v", err)
	} else if count > 0 {
		return ErrUserOwnRepos{UID: u.ID}
	}

	// Check membership of organization.
	count, err = u.getOrganizationCount(e)
	if err != nil {
		return fmt.Errorf("GetOrganizationCount: %v", err)
	} else if count > 0 {
		return ErrUserHasOrgs{UID: u.ID}
	}

	// ***** START: Watch *****
	watches := make([]*Watch, 0, 10)
	if err = e.Find(&watches, &Watch{UserID: u.ID}); err != nil {
		return fmt.Errorf("get all watches: %v", err)
	}
	for i := range watches {
		if _, err = e.Exec("UPDATE `repository` SET num_watches=num_watches-1 WHERE id=?", watches[i].RepoID); err != nil {
			return fmt.Errorf("decrease repository watch number[%d]: %v", watches[i].RepoID, err)
		}
	}
	// ***** END: Watch *****

	// ***** START: Star *****
	stars := make([]*Star, 0, 10)
	if err = e.Find(&stars, &Star{UID: u.ID}); err != nil {
		return fmt.Errorf("get all stars: %v", err)
	}
	for i := range stars {
		if _, err = e.Exec("UPDATE `repository` SET num_stars=num_stars-1 WHERE id=?", stars[i].RepoID); err != nil {
			return fmt.Errorf("decrease repository star number[%d]: %v", stars[i].RepoID, err)
		}
	}
	// ***** END: Star *****

	// ***** START: Follow *****
	followers := make([]*Follow, 0, 10)
	if err = e.Find(&followers, &Follow{UserID: u.ID}); err != nil {
		return fmt.Errorf("get all followers: %v", err)
	}
	for i := range followers {
		if _, err = e.Exec("UPDATE `user` SET num_followers=num_followers-1 WHERE id=?", followers[i].UserID); err != nil {
			return fmt.Errorf("decrease user follower number[%d]: %v", followers[i].UserID, err)
		}
	}
	// ***** END: Follow *****

	if err = deleteBeans(e,
		&AccessToken{UserID: u.ID},
		&Collaboration{UserID: u.ID},
		&Access{UserID: u.ID},
		&Watch{UserID: u.ID},
		&Star{UID: u.ID},
		&Follow{FollowID: u.ID},
		&Action{UserID: u.ID},
		&IssueUser{UID: u.ID},
		&EmailAddress{UID: u.ID},
	); err != nil {
		return fmt.Errorf("deleteBeans: %v", err)
	}

	// ***** START: PublicKey *****
	keys := make([]*PublicKey, 0, 10)
	if err = e.Find(&keys, &PublicKey{OwnerID: u.ID}); err != nil {
		return fmt.Errorf("get all public keys: %v", err)
	}

	keyIDs := make([]int64, len(keys))
	for i := range keys {
		keyIDs[i] = keys[i].ID
	}
	if err = deletePublicKeys(e, keyIDs...); err != nil {
		return fmt.Errorf("deletePublicKeys: %v", err)
	}
	// ***** END: PublicKey *****

	// Clear assignee.
	if _, err = e.Exec("UPDATE `issue` SET assignee_id=0 WHERE assignee_id=?", u.ID); err != nil {
		return fmt.Errorf("clear assignee: %v", err)
	}

	if _, err = e.ID(u.ID).Delete(new(User)); err != nil {
		return fmt.Errorf("Delete: %v", err)
	}

	// FIXME: system notice
	// Note: There are something just cannot be roll back,
	//	so just keep error logs of those operations.

	_ = os.RemoveAll(UserPath(u.Name))
	_ = os.Remove(u.CustomAvatarPath())

	return nil
}

// DeleteUser completely and permanently deletes everything of a user,
// but issues/comments/pulls will be kept and shown as someone has been deleted.
func DeleteUser(u *User) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = deleteUser(sess, u); err != nil {
		// Note: don't wrapper error here.
		return err
	}

	if err = sess.Commit(); err != nil {
		return err
	}

	return RewriteAuthorizedKeys()
}

// DeleteInactivateUsers deletes all inactivate users and email addresses.
func DeleteInactivateUsers() (err error) {
	users := make([]*User, 0, 10)
	if err = x.Where("is_active = ?", false).Find(&users); err != nil {
		return fmt.Errorf("get all inactive users: %v", err)
	}
	// FIXME: should only update authorized_keys file once after all deletions.
	for _, u := range users {
		if err = DeleteUser(u); err != nil {
			// Ignore users that were set inactive by admin.
			if IsErrUserOwnRepos(err) || IsErrUserHasOrgs(err) {
				continue
			}
			return err
		}
	}

	_, err = x.Where("is_activated = ?", false).Delete(new(EmailAddress))
	return err
}

// UserPath returns the path absolute path of user repositories.
func UserPath(username string) string {
	return filepath.Join(conf.Repository.Root, strings.ToLower(username))
}

func GetUserByKeyID(keyID int64) (*User, error) {
	user := new(User)
	has, err := x.SQL("SELECT a.* FROM `user` AS a, public_key AS b WHERE a.id = b.owner_id AND b.id=?", keyID).Get(user)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, errors.UserNotKeyOwner{KeyID: keyID}
	}
	return user, nil
}

func getUserByID(e Engine, id int64) (*User, error) {
	u := new(User)
	has, err := e.ID(id).Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist{args: map[string]interface{}{"userID": id}}
	}
	return u, nil
}

// GetUserByID returns the user object by given ID if exists.
// Deprecated: Use Users.GetByID instead.
func GetUserByID(id int64) (*User, error) {
	return getUserByID(x, id)
}

// GetAssigneeByID returns the user with read access of repository by given ID.
func GetAssigneeByID(repo *Repository, userID int64) (*User, error) {
	if !Perms.Authorize(userID, repo.ID, AccessModeRead,
		AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		},
	) {
		return nil, ErrUserNotExist{args: map[string]interface{}{"userID": userID}}
	}
	return Users.GetByID(userID)
}

// GetUserByName returns a user by given name.
// Deprecated: Use Users.GetByUsername instead.
func GetUserByName(name string) (*User, error) {
	if len(name) == 0 {
		return nil, ErrUserNotExist{args: map[string]interface{}{"name": name}}
	}
	u := &User{LowerName: strings.ToLower(name)}
	has, err := x.Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist{args: map[string]interface{}{"name": name}}
	}
	return u, nil
}

// GetUserEmailsByNames returns a list of e-mails corresponds to names.
func GetUserEmailsByNames(names []string) []string {
	mails := make([]string, 0, len(names))
	for _, name := range names {
		u, err := GetUserByName(name)
		if err != nil {
			continue
		}
		if u.IsMailable() {
			mails = append(mails, u.Email)
		}
	}
	return mails
}

// GetUserIDsByNames returns a slice of ids corresponds to names.
func GetUserIDsByNames(names []string) []int64 {
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		u, err := GetUserByName(name)
		if err != nil {
			continue
		}
		ids = append(ids, u.ID)
	}
	return ids
}

// UserCommit represents a commit with validation of user.
type UserCommit struct {
	User *User
	*git.Commit
}

// ValidateCommitWithEmail checks if author's e-mail of commit is corresponding to a user.
func ValidateCommitWithEmail(c *git.Commit) *User {
	u, err := GetUserByEmail(c.Author.Email)
	if err != nil {
		return nil
	}
	return u
}

// ValidateCommitsWithEmails checks if authors' e-mails of commits are corresponding to users.
func ValidateCommitsWithEmails(oldCommits []*git.Commit) []*UserCommit {
	emails := make(map[string]*User)
	newCommits := make([]*UserCommit, len(oldCommits))
	for i := range oldCommits {
		var u *User
		if v, ok := emails[oldCommits[i].Author.Email]; !ok {
			u, _ = GetUserByEmail(oldCommits[i].Author.Email)
			emails[oldCommits[i].Author.Email] = u
		} else {
			u = v
		}

		newCommits[i] = &UserCommit{
			User:   u,
			Commit: oldCommits[i],
		}
	}
	return newCommits
}

// GetUserByEmail returns the user object by given e-mail if exists.
// Deprecated: Use Users.GetByEmail instead.
func GetUserByEmail(email string) (*User, error) {
	if len(email) == 0 {
		return nil, ErrUserNotExist{args: map[string]interface{}{"email": email}}
	}

	email = strings.ToLower(email)
	// First try to find the user by primary email
	user := &User{Email: email}
	has, err := x.Get(user)
	if err != nil {
		return nil, err
	}
	if has {
		return user, nil
	}

	// Otherwise, check in alternative list for activated email addresses
	emailAddress := &EmailAddress{Email: email, IsActivated: true}
	has, err = x.Get(emailAddress)
	if err != nil {
		return nil, err
	}
	if has {
		return GetUserByID(emailAddress.UID)
	}

	return nil, ErrUserNotExist{args: map[string]interface{}{"email": email}}
}

type SearchUserOptions struct {
	Keyword  string
	Type     UserType
	OrderBy  string
	Page     int
	PageSize int // Can be smaller than or equal to setting.UI.ExplorePagingNum
}

// SearchUserByName takes keyword and part of user name to search,
// it returns results in given range and number of total results.
func SearchUserByName(opts *SearchUserOptions) (users []*User, _ int64, _ error) {
	if len(opts.Keyword) == 0 {
		return users, 0, nil
	}
	opts.Keyword = strings.ToLower(opts.Keyword)

	if opts.PageSize <= 0 || opts.PageSize > conf.UI.ExplorePagingNum {
		opts.PageSize = conf.UI.ExplorePagingNum
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}

	searchQuery := "%" + opts.Keyword + "%"
	users = make([]*User, 0, opts.PageSize)
	// Append conditions
	sess := x.Where("LOWER(lower_name) LIKE ?", searchQuery).
		Or("LOWER(full_name) LIKE ?", searchQuery).
		And("type = ?", opts.Type)

	countSess := *sess
	count, err := countSess.Count(new(User))
	if err != nil {
		return nil, 0, fmt.Errorf("Count: %v", err)
	}

	if len(opts.OrderBy) > 0 {
		sess.OrderBy(opts.OrderBy)
	}
	return users, count, sess.Limit(opts.PageSize, (opts.Page-1)*opts.PageSize).Find(&users)
}

// ___________    .__  .__
// \_   _____/___ |  | |  |   ______  _  __
//  |    __)/  _ \|  | |  |  /  _ \ \/ \/ /
//  |     \(  <_> )  |_|  |_(  <_> )     /
//  \___  / \____/|____/____/\____/ \/\_/
//      \/

// Follow represents relations of user and his/her followers.
type Follow struct {
	ID       int64
	UserID   int64 `xorm:"UNIQUE(follow)"`
	FollowID int64 `xorm:"UNIQUE(follow)"`
}

func IsFollowing(userID, followID int64) bool {
	has, _ := x.Get(&Follow{UserID: userID, FollowID: followID})
	return has
}

// FollowUser marks someone be another's follower.
func FollowUser(userID, followID int64) (err error) {
	if userID == followID || IsFollowing(userID, followID) {
		return nil
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(&Follow{UserID: userID, FollowID: followID}); err != nil {
		return err
	}

	if _, err = sess.Exec("UPDATE `user` SET num_followers = num_followers + 1 WHERE id = ?", followID); err != nil {
		return err
	}

	if _, err = sess.Exec("UPDATE `user` SET num_following = num_following + 1 WHERE id = ?", userID); err != nil {
		return err
	}
	return sess.Commit()
}

// UnfollowUser unmarks someone be another's follower.
func UnfollowUser(userID, followID int64) (err error) {
	if userID == followID || !IsFollowing(userID, followID) {
		return nil
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Delete(&Follow{UserID: userID, FollowID: followID}); err != nil {
		return err
	}

	if _, err = sess.Exec("UPDATE `user` SET num_followers = num_followers - 1 WHERE id = ?", followID); err != nil {
		return err
	}

	if _, err = sess.Exec("UPDATE `user` SET num_following = num_following - 1 WHERE id = ?", userID); err != nil {
		return err
	}
	return sess.Commit()
}

// GetRepositoryAccesses finds all repositories with their access mode where a user has access but does not own.
func (u *User) GetRepositoryAccesses() (map[*Repository]AccessMode, error) {
	accesses := make([]*Access, 0, 10)
	if err := x.Find(&accesses, &Access{UserID: u.ID}); err != nil {
		return nil, err
	}

	repos := make(map[*Repository]AccessMode, len(accesses))
	for _, access := range accesses {
		repo, err := GetRepositoryByID(access.RepoID)
		if err != nil {
			if IsErrRepoNotExist(err) {
				log.Error("Failed to get repository by ID: %v", err)
				continue
			}
			return nil, err
		}
		if repo.OwnerID == u.ID {
			continue
		}
		repos[repo] = access.Mode
	}
	return repos, nil
}

// GetAccessibleRepositories finds repositories which the user has access but does not own.
// If limit is smaller than 1 means returns all found results.
func (user *User) GetAccessibleRepositories(limit int) (repos []*Repository, _ error) {
	sess := x.Where("owner_id !=? ", user.ID).Desc("updated_unix")
	if limit > 0 {
		sess.Limit(limit)
		repos = make([]*Repository, 0, limit)
	} else {
		repos = make([]*Repository, 0, 10)
	}
	return repos, sess.Join("INNER", "access", "access.user_id = ? AND access.repo_id = repository.id", user.ID).Find(&repos)
}
