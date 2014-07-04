// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

type UserType int

const (
	INDIVIDUAL UserType = iota // Historic reason to make it starts at 0.
	ORGANIZATION
)

var (
	ErrUserOwnRepos          = errors.New("User still have ownership of repositories")
	ErrUserHasOrgs           = errors.New("User still have membership of organization")
	ErrUserAlreadyExist      = errors.New("User already exist")
	ErrUserNotExist          = errors.New("User does not exist")
	ErrUserNotKeyOwner       = errors.New("User does not the owner of public key")
	ErrEmailAlreadyUsed      = errors.New("E-mail already used")
	ErrUserNameIllegal       = errors.New("User name contains illegal characters")
	ErrLoginSourceNotExist   = errors.New("Login source does not exist")
	ErrLoginSourceNotActived = errors.New("Login source is not actived")
	ErrUnsupportedLoginType  = errors.New("Login source is unknown")
)

// User represents the object of individual and member of organization.
type User struct {
	Id            int64
	LowerName     string `xorm:"unique not null"`
	Name          string `xorm:"unique not null"`
	FullName      string
	Email         string `xorm:"unique not null"`
	Passwd        string `xorm:"not null"`
	LoginType     LoginType
	LoginSource   int64 `xorm:"not null default 0"`
	LoginName     string
	Type          UserType
	Orgs          []*User `xorm:"-"`
	NumFollowers  int
	NumFollowings int
	NumStars      int
	NumRepos      int
	Avatar        string `xorm:"varchar(2048) not null"`
	AvatarEmail   string `xorm:"not null"`
	Location      string
	Website       string
	IsActive      bool
	IsAdmin       bool
	Rands         string    `xorm:"VARCHAR(10)"`
	Salt          string    `xorm:"VARCHAR(10)"`
	Created       time.Time `xorm:"created"`
	Updated       time.Time `xorm:"updated"`

	// For organization.
	Description string
	NumTeams    int
	NumMembers  int
	Teams       []*Team `xorm:"-"`
	Members     []*User `xorm:"-"`
}

// HomeLink returns the user home page link.
func (u *User) HomeLink() string {
	return "/user/" + u.Name
}

// AvatarLink returns user gravatar link.
func (u *User) AvatarLink() string {
	if setting.DisableGravatar {
		return "/img/avatar_default.jpg"
	} else if setting.Service.EnableCacheAvatar {
		return "/avatar/" + u.Avatar
	}
	return "//1.gravatar.com/avatar/" + u.Avatar
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

// IsOrganization returns true if user is actually a organization.
func (u *User) IsOrganization() bool {
	return u.Type == ORGANIZATION
}

// GetOrganizationCount returns count of membership of organization of user.
func (u *User) GetOrganizationCount() (int64, error) {
	return x.Where("uid=?", u.Id).Count(new(OrgUser))
}

// GetOrganizations returns all organizations that user belongs to.
func (u *User) GetOrganizations() error {
	ous, err := GetOrgUsersByUserId(u.Id)
	if err != nil {
		return err
	}

	u.Orgs = make([]*User, len(ous))
	for i, ou := range ous {
		u.Orgs[i], err = GetUserById(ou.OrgId)
		if err != nil {
			return err
		}
	}
	return nil
}

// IsUserExist checks if given user name exist,
// the user name should be noncased unique.
func IsUserExist(name string) (bool, error) {
	if len(name) == 0 {
		return false, nil
	}
	return x.Get(&User{LowerName: strings.ToLower(name)})
}

// IsEmailUsed returns true if the e-mail has been used.
func IsEmailUsed(email string) (bool, error) {
	if len(email) == 0 {
		return false, nil
	}
	return x.Get(&User{Email: email})
}

// GetUserSalt returns a user salt token
func GetUserSalt() string {
	return base.GetRandomString(10)
}

// CreateUser creates record of a new user.
func CreateUser(u *User) (*User, error) {
	if !IsLegalName(u.Name) {
		return nil, ErrUserNameIllegal
	}

	isExist, err := IsUserExist(u.Name)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrUserAlreadyExist
	}

	isExist, err = IsEmailUsed(u.Email)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrEmailAlreadyUsed
	}

	u.LowerName = strings.ToLower(u.Name)
	u.Avatar = base.EncodeMd5(u.Email)
	u.AvatarEmail = u.Email
	u.Rands = GetUserSalt()
	u.Salt = GetUserSalt()
	u.EncodePasswd()

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if _, err = sess.Insert(u); err != nil {
		sess.Rollback()
		return nil, err
	}

	if err = os.MkdirAll(UserPath(u.Name), os.ModePerm); err != nil {
		sess.Rollback()
		return nil, err
	}

	if err = sess.Commit(); err != nil {
		return nil, err
	}

	// Auto-set admin for user whose ID is 1.
	if u.Id == 1 {
		u.IsAdmin = true
		u.IsActive = true
		_, err = x.Id(u.Id).UseBool().Update(u)
	}
	return u, err
}

// GetUsers returns given number of user objects with offset.
func GetUsers(num, offset int) ([]User, error) {
	users := make([]User, 0, num)
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
		log.Error("user.getVerifyUser: %v", err)
	}

	return nil
}

// verify active code when active account
func VerifyUserActiveCode(code string) (user *User) {
	minutes := setting.Service.ActiveCodeLives

	if user = getVerifyUser(code); user != nil {
		// time limit code
		prefix := code[:base.TimeLimitCodeLength]
		data := base.ToStr(user.Id) + user.Email + user.LowerName + user.Passwd + user.Rands

		if base.VerifyTimeLimitCode(data, minutes, prefix) {
			return user
		}
	}
	return nil
}

// ChangeUserName changes all corresponding setting from old user name to new one.
func ChangeUserName(user *User, newUserName string) (err error) {
	newUserName = strings.ToLower(newUserName)

	// Update accesses of user.
	accesses := make([]Access, 0, 10)
	if err = x.Find(&accesses, &Access{UserName: user.LowerName}); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for i := range accesses {
		accesses[i].UserName = newUserName
		if strings.HasPrefix(accesses[i].RepoName, user.LowerName+"/") {
			accesses[i].RepoName = strings.Replace(accesses[i].RepoName, user.LowerName, newUserName, 1)
		}
		if err = UpdateAccessWithSession(sess, &accesses[i]); err != nil {
			return err
		}
	}

	repos, err := GetRepositories(user.Id, true)
	if err != nil {
		return err
	}
	for i := range repos {
		accesses = make([]Access, 0, 10)
		// Update accesses of user repository.
		if err = x.Find(&accesses, &Access{RepoName: user.LowerName + "/" + repos[i].LowerName}); err != nil {
			return err
		}

		for j := range accesses {
			accesses[j].UserName = newUserName
			accesses[j].RepoName = newUserName + "/" + repos[i].LowerName
			if err = UpdateAccessWithSession(sess, &accesses[j]); err != nil {
				return err
			}
		}
	}

	// Change user directory name.
	if err = os.Rename(UserPath(user.LowerName), UserPath(newUserName)); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

// UpdateUser updates user's information.
func UpdateUser(u *User) (err error) {
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

	_, err = x.Id(u.Id).AllCols().Update(u)
	return err
}

// TODO: need some kind of mechanism to record failure.
// DeleteUser completely and permanently deletes everything of user.
func DeleteUser(u *User) error {
	// Check ownership of repository.
	count, err := GetRepositoryCount(u)
	if err != nil {
		return errors.New("modesl.GetRepositories(GetRepositoryCount): " + err.Error())
	} else if count > 0 {
		return ErrUserOwnRepos
	}

	// Check membership of organization.
	count, err = u.GetOrganizationCount()
	if err != nil {
		return errors.New("modesl.GetRepositories(GetOrganizationCount): " + err.Error())
	} else if count > 0 {
		return ErrUserHasOrgs
	}

	// TODO: check issues, other repos' commits
	// TODO: roll backable in some point.

	// Delete all followers.
	if _, err = x.Delete(&Follow{FollowId: u.Id}); err != nil {
		return err
	}
	// Delete oauth2.
	if _, err = x.Delete(&Oauth2{Uid: u.Id}); err != nil {
		return err
	}
	// Delete all feeds.
	if _, err = x.Delete(&Action{UserId: u.Id}); err != nil {
		return err
	}
	// Delete all watches.
	if _, err = x.Delete(&Watch{UserId: u.Id}); err != nil {
		return err
	}
	// Delete all accesses.
	if _, err = x.Delete(&Access{UserName: u.LowerName}); err != nil {
		return err
	}
	// Delete all SSH keys.
	keys := make([]*PublicKey, 0, 10)
	if err = x.Find(&keys, &PublicKey{OwnerId: u.Id}); err != nil {
		return err
	}
	for _, key := range keys {
		if err = DeletePublicKey(key); err != nil {
			return err
		}
	}

	// Delete user directory.
	if err = os.RemoveAll(UserPath(u.Name)); err != nil {
		return err
	}

	_, err = x.Delete(u)
	return err
}

// DeleteInactivateUsers deletes all inactivate users.
func DeleteInactivateUsers() error {
	_, err := x.Where("is_active=?", false).Delete(new(User))
	return err
}

// UserPath returns the path absolute path of user repositories.
func UserPath(userName string) string {
	return filepath.Join(setting.RepoRootPath, strings.ToLower(userName))
}

func GetUserByKeyId(keyId int64) (*User, error) {
	user := new(User)
	rawSql := "SELECT a.* FROM `user` AS a, public_key AS b WHERE a.id = b.owner_id AND b.id=?"
	has, err := x.Sql(rawSql, keyId).Get(user)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotKeyOwner
	}
	return user, nil
}

// GetUserById returns the user object by given ID if exists.
func GetUserById(id int64) (*User, error) {
	u := new(User)
	has, err := x.Id(id).Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist
	}
	return u, nil
}

// GetUserByName returns the user object by given name if exists.
func GetUserByName(name string) (*User, error) {
	if len(name) == 0 {
		return nil, ErrUserNotExist
	}
	user := &User{LowerName: strings.ToLower(name)}
	has, err := x.Get(user)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist
	}
	return user, nil
}

// GetUserEmailsByNames returns a slice of e-mails corresponds to names.
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

// GetUserByEmail returns the user object by given e-mail if exists.
func GetUserByEmail(email string) (*User, error) {
	if len(email) == 0 {
		return nil, ErrUserNotExist
	}
	user := &User{Email: strings.ToLower(email)}
	has, err := x.Get(user)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist
	}
	return user, nil
}

// SearchUserByName returns given number of users whose name contains keyword.
func SearchUserByName(key string, limit int) (us []*User, err error) {
	// Prevent SQL inject.
	key = strings.TrimSpace(key)
	if len(key) == 0 {
		return us, nil
	}

	key = strings.Split(key, " ")[0]
	if len(key) == 0 {
		return us, nil
	}
	key = strings.ToLower(key)

	us = make([]*User, 0, limit)
	err = x.Limit(limit).Where("lower_name like '%" + key + "%'").Find(&us)
	return us, err
}

// Follow is connection request for receiving user notifycation.
type Follow struct {
	Id       int64
	UserId   int64 `xorm:"unique(follow)"`
	FollowId int64 `xorm:"unique(follow)"`
}

// FollowUser marks someone be another's follower.
func FollowUser(userId int64, followId int64) (err error) {
	session := x.NewSession()
	defer session.Close()
	session.Begin()

	if _, err = session.Insert(&Follow{UserId: userId, FollowId: followId}); err != nil {
		session.Rollback()
		return err
	}

	rawSql := "UPDATE `user` SET num_followers = num_followers + 1 WHERE id = ?"
	if _, err = session.Exec(rawSql, followId); err != nil {
		session.Rollback()
		return err
	}

	rawSql = "UPDATE `user` SET num_followings = num_followings + 1 WHERE id = ?"
	if _, err = session.Exec(rawSql, userId); err != nil {
		session.Rollback()
		return err
	}
	return session.Commit()
}

// UnFollowUser unmarks someone be another's follower.
func UnFollowUser(userId int64, unFollowId int64) (err error) {
	session := x.NewSession()
	defer session.Close()
	session.Begin()

	if _, err = session.Delete(&Follow{UserId: userId, FollowId: unFollowId}); err != nil {
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
