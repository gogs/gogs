// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/nfnt/resize"

	"github.com/gogits/gogs/modules/avatar"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

type UserType int

const (
	INDIVIDUAL UserType = iota // Historic reason to make it starts at 0.
	ORGANIZATION
)

var (
	ErrUserNotKeyOwner       = errors.New("User does not the owner of public key")
	ErrEmailNotExist         = errors.New("E-mail does not exist")
	ErrEmailNotActivated     = errors.New("E-mail address has not been activated")
	ErrUserNameIllegal       = errors.New("User name contains illegal characters")
	ErrLoginSourceNotExist   = errors.New("Login source does not exist")
	ErrLoginSourceNotActived = errors.New("Login source is not actived")
	ErrUnsupportedLoginType  = errors.New("Login source is unknown")
)

// User represents the object of individual and member of organization.
type User struct {
	Id        int64
	LowerName string `xorm:"UNIQUE NOT NULL"`
	Name      string `xorm:"UNIQUE NOT NULL"`
	FullName  string
	// Email is the primary email address (to be used for communication).
	Email       string `xorm:"UNIQUE(s) NOT NULL"`
	Passwd      string `xorm:"NOT NULL"`
	LoginType   LoginType
	LoginSource int64 `xorm:"NOT NULL DEFAULT 0"`
	LoginName   string
	Type        UserType      `xorm:"UNIQUE(s)"`
	Orgs        []*User       `xorm:"-"`
	Repos       []*Repository `xorm:"-"`
	Location    string
	Website     string
	Rands       string    `xorm:"VARCHAR(10)"`
	Salt        string    `xorm:"VARCHAR(10)"`
	Created     time.Time `xorm:"CREATED"`
	Updated     time.Time `xorm:"UPDATED"`

	// Remember visibility choice for convenience.
	LastRepoVisibility bool

	// Permissions.
	IsActive     bool
	IsAdmin      bool
	AllowGitHook bool

	// Avatar.
	Avatar          string `xorm:"VARCHAR(2048) NOT NULL"`
	AvatarEmail     string `xorm:"NOT NULL"`
	UseCustomAvatar bool

	// Counters.
	NumFollowers  int
	NumFollowings int
	NumStars      int
	NumRepos      int

	// For organization.
	Description string
	NumTeams    int
	NumMembers  int
	Teams       []*Team `xorm:"-"`
	Members     []*User `xorm:"-"`
}

// EmailAdresses is the list of all email addresses of a user. Can contain the
// primary email address, but is not obligatory
type EmailAddress struct {
	Id          int64
	Uid         int64  `xorm:"INDEX NOT NULL"`
	Email       string `xorm:"UNIQUE NOT NULL"`
	IsActivated bool
	IsPrimary   bool `xorm:"-"`
}

// DashboardLink returns the user dashboard page link.
func (u *User) DashboardLink() string {
	if u.IsOrganization() {
		return setting.AppSubUrl + "/org/" + u.Name + "/dashboard/"
	}
	return setting.AppSubUrl + "/"
}

// HomeLink returns the user or organization home page link.
func (u *User) HomeLink() string {
	if u.IsOrganization() {
		return setting.AppSubUrl + "/org/" + u.Name
	}
	return setting.AppSubUrl + "/" + u.Name
}

func (u *User) RelAvatarLink() string {
	defaultImgUrl := "/img/avatar_default.jpg"
	if u.Id == -1 {
		return defaultImgUrl
	}

	imgPath := path.Join(setting.AvatarUploadPath, com.ToStr(u.Id))
	switch {
	case u.UseCustomAvatar:
		if !com.IsExist(imgPath) {
			return defaultImgUrl
		}
		return "/avatars/" + com.ToStr(u.Id)
	case setting.DisableGravatar, setting.OfflineMode:
		if !com.IsExist(imgPath) {
			img, err := avatar.RandomImage([]byte(u.Email))
			if err != nil {
				log.Error(3, "RandomImage: %v", err)
				return defaultImgUrl
			}
			if err = os.MkdirAll(path.Dir(imgPath), os.ModePerm); err != nil {
				log.Error(3, "MkdirAll: %v", err)
				return defaultImgUrl
			}
			fw, err := os.Create(imgPath)
			if err != nil {
				log.Error(3, "Create: %v", err)
				return defaultImgUrl
			}
			defer fw.Close()

			if err = jpeg.Encode(fw, img, nil); err != nil {
				log.Error(3, "Encode: %v", err)
				return defaultImgUrl
			}
			log.Info("New random avatar created: %d", u.Id)
		}

		return "/avatars/" + com.ToStr(u.Id)
	case setting.Service.EnableCacheAvatar:
		return "/avatar/" + u.Avatar
	}
	return setting.GravatarSource + u.Avatar
}

// AvatarLink returns user gravatar link.
func (u *User) AvatarLink() string {
	link := u.RelAvatarLink()
	if link[0] == '/' {
		return setting.AppSubUrl + link
	}
	return link
}

// NewGitSig generates and returns the signature of given user.
func (u *User) NewGitSig() *git.Signature {
	return &git.Signature{
		Name:  u.Name,
		Email: u.Email,
		When:  time.Now(),
	}
}

// EncodePasswd encodes password to safe format.
func (u *User) EncodePasswd() {
	newPasswd := base.PBKDF2([]byte(u.Passwd), []byte(u.Salt), 10000, 50, sha256.New)
	u.Passwd = fmt.Sprintf("%x", newPasswd)
}

// ValidatePassword checks if given password matches the one belongs to the user.
func (u *User) ValidatePassword(passwd string) bool {
	newUser := &User{Passwd: passwd, Salt: u.Salt}
	newUser.EncodePasswd()
	return u.Passwd == newUser.Passwd
}

// CustomAvatarPath returns user custom avatar file path.
func (u *User) CustomAvatarPath() string {
	return filepath.Join(setting.AvatarUploadPath, com.ToStr(u.Id))
}

// UploadAvatar saves custom avatar for user.
// FIXME: split uploads to different subdirs in case we have massive users.
func (u *User) UploadAvatar(data []byte) error {
	u.UseCustomAvatar = true

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	m := resize.Resize(234, 234, img, resize.NearestNeighbor)

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Id(u.Id).AllCols().Update(u); err != nil {
		sess.Rollback()
		return err
	}

	os.MkdirAll(setting.AvatarUploadPath, os.ModePerm)
	fw, err := os.Create(u.CustomAvatarPath())
	if err != nil {
		sess.Rollback()
		return err
	}
	defer fw.Close()
	if err = jpeg.Encode(fw, m, nil); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

// IsAdminOfRepo returns true if user has admin or higher access of repository.
func (u *User) IsAdminOfRepo(repo *Repository) bool {
	if err := repo.GetOwner(); err != nil {
		log.Error(3, "GetOwner: %v", err)
		return false
	}

	if repo.Owner.IsOrganization() {
		has, err := HasAccess(u, repo, ACCESS_MODE_ADMIN)
		if err != nil {
			log.Error(3, "HasAccess: %v", err)
			return false
		}
		return has
	}

	return repo.IsOwnedBy(u.Id)
}

// IsOrganization returns true if user is actually a organization.
func (u *User) IsOrganization() bool {
	return u.Type == ORGANIZATION
}

// IsUserOrgOwner returns true if user is in the owner team of given organization.
func (u *User) IsUserOrgOwner(orgId int64) bool {
	return IsOrganizationOwner(orgId, u.Id)
}

// IsPublicMember returns true if user public his/her membership in give organization.
func (u *User) IsPublicMember(orgId int64) bool {
	return IsPublicMembership(orgId, u.Id)
}

// GetOrganizationCount returns count of membership of organization of user.
func (u *User) GetOrganizationCount() (int64, error) {
	return x.Where("uid=?", u.Id).Count(new(OrgUser))
}

// GetRepositories returns all repositories that user owns, including private repositories.
func (u *User) GetRepositories() (err error) {
	u.Repos, err = GetRepositories(u.Id, true)
	return err
}

// GetOrganizations returns all organizations that user belongs to.
func (u *User) GetOrganizations() error {
	ous, err := GetOrgUsersByUserId(u.Id)
	if err != nil {
		return err
	}

	u.Orgs = make([]*User, len(ous))
	for i, ou := range ous {
		u.Orgs[i], err = GetUserByID(ou.OrgID)
		if err != nil {
			return err
		}
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

// IsUserExist checks if given user name exist,
// the user name should be noncased unique.
// If uid is presented, then check will rule out that one,
// it is used when update a user name in settings page.
func IsUserExist(uid int64, name string) (bool, error) {
	if len(name) == 0 {
		return false, nil
	}
	return x.Where("id!=?", uid).Get(&User{LowerName: strings.ToLower(name)})
}

// IsEmailUsed returns true if the e-mail has been used.
func IsEmailUsed(email string) (bool, error) {
	if len(email) == 0 {
		return false, nil
	}

	email = strings.ToLower(email)
	if has, err := x.Get(&EmailAddress{Email: email}); has || err != nil {
		return has, err
	}
	return x.Get(&User{Email: email})
}

// GetUserSalt returns a ramdom user salt token.
func GetUserSalt() string {
	return base.GetRandomString(10)
}

// NewFakeUser creates and returns a fake user for someone has deleted his/her account.
func NewFakeUser() *User {
	return &User{
		Id:        -1,
		Name:      "Someone",
		LowerName: "someone",
	}
}

// CreateUser creates record of a new user.
func CreateUser(u *User) (err error) {
	if err = IsUsableName(u.Name); err != nil {
		return err
	}

	isExist, err := IsUserExist(0, u.Name)
	if err != nil {
		return err
	} else if isExist {
		return ErrUserAlreadyExist{u.Name}
	}

	isExist, err = IsEmailUsed(u.Email)
	if err != nil {
		return err
	} else if isExist {
		return ErrEmailAlreadyUsed{u.Email}
	}

	u.LowerName = strings.ToLower(u.Name)
	u.AvatarEmail = u.Email
	u.Avatar = avatar.HashEmail(u.AvatarEmail)
	u.Rands = GetUserSalt()
	u.Salt = GetUserSalt()
	u.EncodePasswd()

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(u); err != nil {
		sess.Rollback()
		return err
	} else if err = os.MkdirAll(UserPath(u.Name), os.ModePerm); err != nil {
		sess.Rollback()
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

// GetUsers returns given number of user objects with offset.
func GetUsers(num, offset int) ([]*User, error) {
	users := make([]*User, 0, num)
	err := x.Limit(num, offset).Where("type=0").Asc("id").Find(&users)
	return users, err
}

// get user by erify code
func getVerifyUser(code string) (user *User) {
	if len(code) <= base.TimeLimitCodeLength {
		return nil
	}

	// use tail hex username query user
	hexStr := code[base.TimeLimitCodeLength:]
	if b, err := hex.DecodeString(hexStr); err == nil {
		if user, err = GetUserByName(string(b)); user != nil {
			return user
		}
		log.Error(4, "user.getVerifyUser: %v", err)
	}

	return nil
}

// verify active code when active account
func VerifyUserActiveCode(code string) (user *User) {
	minutes := setting.Service.ActiveCodeLives

	if user = getVerifyUser(code); user != nil {
		// time limit code
		prefix := code[:base.TimeLimitCodeLength]
		data := com.ToStr(user.Id) + user.Email + user.LowerName + user.Passwd + user.Rands

		if base.VerifyTimeLimitCode(data, minutes, prefix) {
			return user
		}
	}
	return nil
}

// verify active code when active account
func VerifyActiveEmailCode(code, email string) *EmailAddress {
	minutes := setting.Service.ActiveCodeLives

	if user := getVerifyUser(code); user != nil {
		// time limit code
		prefix := code[:base.TimeLimitCodeLength]
		data := com.ToStr(user.Id) + email + user.LowerName + user.Passwd + user.Rands

		if base.VerifyTimeLimitCode(data, minutes, prefix) {
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
	if err = IsUsableName(newUserName); err != nil {
		return err
	}

	isExist, err := IsUserExist(0, newUserName)
	if err != nil {
		return err
	} else if isExist {
		return ErrUserAlreadyExist{newUserName}
	}

	return os.Rename(UserPath(u.LowerName), UserPath(newUserName))
}

// UpdateUser updates user's information.
func UpdateUser(u *User) error {
	u.Email = strings.ToLower(u.Email)
	has, err := x.Where("id!=?", u.Id).And("type=?", u.Type).And("email=?", u.Email).Get(new(User))
	if err != nil {
		return err
	} else if has {
		return ErrEmailAlreadyUsed{u.Email}
	}

	u.LowerName = strings.ToLower(u.Name)

	if len(u.Location) > 255 {
		u.Location = u.Location[:255]
	}
	if len(u.Website) > 255 {
		u.Website = u.Website[:255]
	}
	if len(u.Description) > 255 {
		u.Description = u.Description[:255]
	}

	if u.AvatarEmail == "" {
		u.AvatarEmail = u.Email
	}
	u.Avatar = avatar.HashEmail(u.AvatarEmail)

	u.FullName = base.Sanitizer.Sanitize(u.FullName)
	_, err = x.Id(u.Id).AllCols().Update(u)
	return err
}

// DeleteBeans deletes all given beans, beans should contain delete conditions.
func DeleteBeans(e Engine, beans ...interface{}) (err error) {
	for i := range beans {
		if _, err = e.Delete(beans[i]); err != nil {
			return err
		}
	}
	return nil
}

// FIXME: need some kind of mechanism to record failure. HINT: system notice
// DeleteUser completely and permanently deletes everything of a user,
// but issues/comments/pulls will be kept and shown as someone has been deleted.
func DeleteUser(u *User) error {
	// Note: A user owns any repository or belongs to any organization
	//	cannot perform delete operation.

	// Check ownership of repository.
	count, err := GetRepositoryCount(u)
	if err != nil {
		return fmt.Errorf("GetRepositoryCount: %v", err)
	} else if count > 0 {
		return ErrUserOwnRepos{UID: u.Id}
	}

	// Check membership of organization.
	count, err = u.GetOrganizationCount()
	if err != nil {
		return fmt.Errorf("GetOrganizationCount: %v", err)
	} else if count > 0 {
		return ErrUserHasOrgs{UID: u.Id}
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	// ***** START: Watch *****
	watches := make([]*Watch, 0, 10)
	if err = x.Find(&watches, &Watch{UserID: u.Id}); err != nil {
		return fmt.Errorf("get all watches: %v", err)
	}
	for i := range watches {
		if _, err = sess.Exec("UPDATE `repository` SET num_watches=num_watches-1 WHERE id=?", watches[i].RepoID); err != nil {
			return fmt.Errorf("decrease repository watch number[%d]: %v", watches[i].RepoID, err)
		}
	}
	// ***** END: Watch *****

	// ***** START: Star *****
	stars := make([]*Star, 0, 10)
	if err = x.Find(&stars, &Star{UID: u.Id}); err != nil {
		return fmt.Errorf("get all stars: %v", err)
	}
	for i := range stars {
		if _, err = sess.Exec("UPDATE `repository` SET num_stars=num_stars-1 WHERE id=?", stars[i].RepoID); err != nil {
			return fmt.Errorf("decrease repository star number[%d]: %v", stars[i].RepoID, err)
		}
	}
	// ***** END: Star *****

	// ***** START: Follow *****
	followers := make([]*Follow, 0, 10)
	if err = x.Find(&followers, &Follow{UserID: u.Id}); err != nil {
		return fmt.Errorf("get all followers: %v", err)
	}
	for i := range followers {
		if _, err = sess.Exec("UPDATE `user` SET num_followers=num_followers-1 WHERE id=?", followers[i].UserID); err != nil {
			return fmt.Errorf("decrease user follower number[%d]: %v", followers[i].UserID, err)
		}
	}
	// ***** END: Follow *****

	if err = DeleteBeans(sess,
		&Oauth2{Uid: u.Id},
		&AccessToken{UID: u.Id},
		&Collaboration{UserID: u.Id},
		&Access{UserID: u.Id},
		&Watch{UserID: u.Id},
		&Star{UID: u.Id},
		&Follow{FollowID: u.Id},
		&Action{UserID: u.Id},
		&IssueUser{UID: u.Id},
		&EmailAddress{Uid: u.Id},
	); err != nil {
		return fmt.Errorf("DeleteBeans: %v", err)
	}

	// ***** START: PublicKey *****
	keys := make([]*PublicKey, 0, 10)
	if err = sess.Find(&keys, &PublicKey{OwnerID: u.Id}); err != nil {
		return fmt.Errorf("get all public keys: %v", err)
	}
	for _, key := range keys {
		if err = deletePublicKey(sess, key.ID); err != nil {
			return fmt.Errorf("deletePublicKey: %v", err)
		}
	}
	// ***** END: PublicKey *****

	// Clear assignee.
	if _, err = sess.Exec("UPDATE `issue` SET assignee_id=0 WHERE assignee_id=?", u.Id); err != nil {
		return fmt.Errorf("clear assignee: %v", err)
	}

	if _, err = sess.Delete(u); err != nil {
		return fmt.Errorf("Delete: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	// FIXME: system notice
	// Note: There are something just cannot be roll back,
	//	so just keep error logs of those operations.

	RewriteAllPublicKeys()
	os.RemoveAll(UserPath(u.Name))
	os.Remove(u.CustomAvatarPath())

	return nil
}

// DeleteInactivateUsers deletes all inactivate users and email addresses.
func DeleteInactivateUsers() (err error) {
	users := make([]*User, 0, 10)
	if err = x.Where("is_active=?", false).Find(&users); err != nil {
		return fmt.Errorf("get all inactive users: %v", err)
	}
	for _, u := range users {
		if err = DeleteUser(u); err != nil {
			// Ignore users that were set inactive by admin.
			if IsErrUserOwnRepos(err) || IsErrUserHasOrgs(err) {
				continue
			}
			return err
		}
	}

	_, err = x.Where("is_activated=?", false).Delete(new(EmailAddress))
	return err
}

// UserPath returns the path absolute path of user repositories.
func UserPath(userName string) string {
	return filepath.Join(setting.RepoRootPath, strings.ToLower(userName))
}

func GetUserByKeyId(keyId int64) (*User, error) {
	user := new(User)
	has, err := x.Sql("SELECT a.* FROM `user` AS a, public_key AS b WHERE a.id = b.owner_id AND b.id=?", keyId).Get(user)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotKeyOwner
	}
	return user, nil
}

func getUserByID(e Engine, id int64) (*User, error) {
	u := new(User)
	has, err := e.Id(id).Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist{id, ""}
	}
	return u, nil
}

// GetUserByID returns the user object by given ID if exists.
func GetUserByID(id int64) (*User, error) {
	return getUserByID(x, id)
}

// GetAssigneeByID returns the user with write access of repository by given ID.
func GetAssigneeByID(repo *Repository, userID int64) (*User, error) {
	has, err := HasAccess(&User{Id: userID}, repo, ACCESS_MODE_WRITE)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist{userID, ""}
	}
	return GetUserByID(userID)
}

// GetUserByName returns user by given name.
func GetUserByName(name string) (*User, error) {
	if len(name) == 0 {
		return nil, ErrUserNotExist{0, name}
	}
	u := &User{LowerName: strings.ToLower(name)}
	has, err := x.Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist{0, name}
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
		mails = append(mails, u.Email)
	}
	return mails
}

// GetUserIdsByNames returns a slice of ids corresponds to names.
func GetUserIdsByNames(names []string) []int64 {
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		u, err := GetUserByName(name)
		if err != nil {
			continue
		}
		ids = append(ids, u.Id)
	}
	return ids
}

// GetEmailAddresses returns all e-mail addresses belongs to given user.
func GetEmailAddresses(uid int64) ([]*EmailAddress, error) {
	emails := make([]*EmailAddress, 0, 5)
	err := x.Where("uid=?", uid).Find(&emails)
	if err != nil {
		return nil, err
	}

	u, err := GetUserByID(uid)
	if err != nil {
		return nil, err
	}

	isPrimaryFound := false
	for _, email := range emails {
		if email.Email == u.Email {
			isPrimaryFound = true
			email.IsPrimary = true
		} else {
			email.IsPrimary = false
		}
	}

	// We alway want the primary email address displayed, even if it's not in
	// the emailaddress table (yet)
	if !isPrimaryFound {
		emails = append(emails, &EmailAddress{
			Email:       u.Email,
			IsActivated: true,
			IsPrimary:   true,
		})
	}
	return emails, nil
}

func AddEmailAddress(email *EmailAddress) error {
	email.Email = strings.ToLower(email.Email)
	used, err := IsEmailUsed(email.Email)
	if err != nil {
		return err
	} else if used {
		return ErrEmailAlreadyUsed{email.Email}
	}

	_, err = x.Insert(email)
	return err
}

func (email *EmailAddress) Activate() error {
	email.IsActivated = true
	if _, err := x.Id(email.Id).AllCols().Update(email); err != nil {
		return err
	}

	if user, err := GetUserByID(email.Uid); err != nil {
		return err
	} else {
		user.Rands = GetUserSalt()
		return UpdateUser(user)
	}
}

func DeleteEmailAddress(email *EmailAddress) error {
	has, err := x.Get(email)
	if err != nil {
		return err
	} else if !has {
		return ErrEmailNotExist
	}

	if _, err = x.Id(email.Id).Delete(email); err != nil {
		return err
	}

	return nil

}

func MakeEmailPrimary(email *EmailAddress) error {
	has, err := x.Get(email)
	if err != nil {
		return err
	} else if !has {
		return ErrEmailNotExist
	}

	if !email.IsActivated {
		return ErrEmailNotActivated
	}

	user := &User{Id: email.Uid}
	has, err = x.Get(user)
	if err != nil {
		return err
	} else if !has {
		return ErrUserNotExist{email.Uid, ""}
	}

	// Make sure the former primary email doesn't disappear
	former_primary_email := &EmailAddress{Email: user.Email}
	has, err = x.Get(former_primary_email)
	if err != nil {
		return err
	} else if !has {
		former_primary_email.Uid = user.Id
		former_primary_email.IsActivated = user.IsActive
		x.Insert(former_primary_email)
	}

	user.Email = email.Email
	_, err = x.Id(user.Id).AllCols().Update(user)

	return err
}

// UserCommit represents a commit with validation of user.
type UserCommit struct {
	User *User
	*git.Commit
}

// ValidateCommitWithEmail chceck if author's e-mail of commit is corresponsind to a user.
func ValidateCommitWithEmail(c *git.Commit) *User {
	u, err := GetUserByEmail(c.Author.Email)
	if err != nil {
		return nil
	}
	return u
}

// ValidateCommitsWithEmails checks if authors' e-mails of commits are corresponding to users.
func ValidateCommitsWithEmails(oldCommits *list.List) *list.List {
	var (
		u          *User
		emails     = map[string]*User{}
		newCommits = list.New()
		e          = oldCommits.Front()
	)
	for e != nil {
		c := e.Value.(*git.Commit)

		if v, ok := emails[c.Author.Email]; !ok {
			u, _ = GetUserByEmail(c.Author.Email)
			emails[c.Author.Email] = u
		} else {
			u = v
		}

		newCommits.PushBack(UserCommit{
			User:   u,
			Commit: c,
		})
		e = e.Next()
	}
	return newCommits
}

// GetUserByEmail returns the user object by given e-mail if exists.
func GetUserByEmail(email string) (*User, error) {
	if len(email) == 0 {
		return nil, ErrUserNotExist{0, "email"}
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
		return GetUserByID(emailAddress.Uid)
	}

	return nil, ErrUserNotExist{0, "email"}
}

// SearchUserByName returns given number of users whose name contains keyword.
func SearchUserByName(opt SearchOption) (us []*User, err error) {
	if len(opt.Keyword) == 0 {
		return us, nil
	}
	opt.Keyword = strings.ToLower(opt.Keyword)

	us = make([]*User, 0, opt.Limit)
	err = x.Limit(opt.Limit).Where("type=0").And("lower_name like ?", "%"+opt.Keyword+"%").Find(&us)
	return us, err
}

// Follow is connection request for receiving user notification.
type Follow struct {
	ID       int64 `xorm:"pk autoincr"`
	UserID   int64 `xorm:"UNIQUE(follow)"`
	FollowID int64 `xorm:"UNIQUE(follow)"`
}

// FollowUser marks someone be another's follower.
func FollowUser(userId int64, followId int64) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	sess.Begin()

	if _, err = sess.Insert(&Follow{UserID: userId, FollowID: followId}); err != nil {
		sess.Rollback()
		return err
	}

	rawSql := "UPDATE `user` SET num_followers = num_followers + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, followId); err != nil {
		sess.Rollback()
		return err
	}

	rawSql = "UPDATE `user` SET num_followings = num_followings + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, userId); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

// UnFollowUser unmarks someone be another's follower.
func UnFollowUser(userId int64, unFollowId int64) (err error) {
	session := x.NewSession()
	defer session.Close()
	session.Begin()

	if _, err = session.Delete(&Follow{UserID: userId, FollowID: unFollowId}); err != nil {
		session.Rollback()
		return err
	}

	rawSql := "UPDATE `user` SET num_followers = num_followers - 1 WHERE id = ?"
	if _, err = session.Exec(rawSql, unFollowId); err != nil {
		session.Rollback()
		return err
	}

	rawSql = "UPDATE `user` SET num_followings = num_followings - 1 WHERE id = ?"
	if _, err = session.Exec(rawSql, userId); err != nil {
		session.Rollback()
		return err
	}
	return session.Commit()
}

func UpdateMentions(userNames []string, issueId int64) error {
	for i := range userNames {
		userNames[i] = strings.ToLower(userNames[i])
	}
	users := make([]*User, 0, len(userNames))

	if err := x.Where("lower_name IN (?)", strings.Join(userNames, "\",\"")).OrderBy("lower_name ASC").Find(&users); err != nil {
		return err
	}

	ids := make([]int64, 0, len(userNames))
	for _, user := range users {
		ids = append(ids, user.Id)
		if !user.IsOrganization() {
			continue
		}

		if user.NumMembers == 0 {
			continue
		}

		tempIds := make([]int64, 0, user.NumMembers)
		orgUsers, err := GetOrgUsersByOrgId(user.Id)
		if err != nil {
			return err
		}

		for _, orgUser := range orgUsers {
			tempIds = append(tempIds, orgUser.ID)
		}

		ids = append(ids, tempIds...)
	}

	if err := UpdateIssueUsersByMentions(ids, issueId); err != nil {
		return err
	}

	return nil
}
