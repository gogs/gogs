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
)

// User types.
const (
	UT_INDIVIDUAL = iota + 1
	UT_ORGANIZATION
)

// Login types.
const (
	LT_PLAIN = iota + 1
	LT_LDAP
)

var (
	ErrUserOwnRepos     = errors.New("User still have ownership of repositories")
	ErrUserAlreadyExist = errors.New("User already exist")
	ErrUserNotExist     = errors.New("User does not exist")
	ErrEmailAlreadyUsed = errors.New("E-mail already used")
	ErrUserNameIllegal  = errors.New("User name contains illegal characters")
	ErrKeyNotExist      = errors.New("Public key does not exist")
)

// User represents the object of individual and member of organization.
type User struct {
	Id            int64
	LowerName     string `xorm:"unique not null"`
	Name          string `xorm:"unique not null"`
	Email         string `xorm:"unique not null"`
	Passwd        string `xorm:"not null"`
	LoginType     int
	Type          int
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
}

// HomeLink returns the user home page link.
func (user *User) HomeLink() string {
	return "/user/" + user.Name
}

// AvatarLink returns the user gravatar link.
func (user *User) AvatarLink() string {
	if base.Service.EnableCacheAvatar {
		return "/avatar/" + user.Avatar
	}
	return "//1.gravatar.com/avatar/" + user.Avatar
}

// NewGitSig generates and returns the signature of given user.
func (user *User) NewGitSig() *git.Signature {
	return &git.Signature{
		Name:  user.Name,
		Email: user.Email,
		When:  time.Now(),
	}
}

// EncodePasswd encodes password to safe format.
func (user *User) EncodePasswd() {
	newPasswd := base.PBKDF2([]byte(user.Passwd), []byte(user.Salt), 10000, 50, sha256.New)
	user.Passwd = fmt.Sprintf("%x", newPasswd)
}

// Member represents user is member of organization.
type Member struct {
	Id     int64
	OrgId  int64 `xorm:"unique(member) index"`
	UserId int64 `xorm:"unique(member)"`
}

// IsUserExist checks if given user name exist,
// the user name should be noncased unique.
func IsUserExist(name string) (bool, error) {
	if len(name) == 0 {
		return false, nil
	}
	return orm.Get(&User{LowerName: strings.ToLower(name)})
}

// IsEmailUsed returns true if the e-mail has been used.
func IsEmailUsed(email string) (bool, error) {
	if len(email) == 0 {
		return false, nil
	}
	return orm.Get(&User{Email: email})
}

// return a user salt token
func GetUserSalt() string {
	return base.GetRandomString(10)
}

// RegisterUser creates record of a new user.
func RegisterUser(user *User) (*User, error) {
	if !IsLegalName(user.Name) {
		return nil, ErrUserNameIllegal
	}

	isExist, err := IsUserExist(user.Name)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrUserAlreadyExist
	}

	isExist, err = IsEmailUsed(user.Email)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrEmailAlreadyUsed
	}

	user.LowerName = strings.ToLower(user.Name)
	user.Avatar = base.EncodeMd5(user.Email)
	user.AvatarEmail = user.Email
	user.Rands = GetUserSalt()
	user.Salt = GetUserSalt()
	user.EncodePasswd()
	if _, err = orm.Insert(user); err != nil {
		return nil, err
	} else if err = os.MkdirAll(UserPath(user.Name), os.ModePerm); err != nil {
		if _, err := orm.Id(user.Id).Delete(&User{}); err != nil {
			return nil, errors.New(fmt.Sprintf(
				"both create userpath %s and delete table record faild: %v", user.Name, err))
		}
		return nil, err
	}

	if user.Id == 1 {
		user.IsAdmin = true
		user.IsActive = true
		_, err = orm.Id(user.Id).UseBool().Update(user)
	}
	return user, err
}

// GetUsers returns given number of user objects with offset.
func GetUsers(num, offset int) ([]User, error) {
	users := make([]User, 0, num)
	err := orm.Limit(num, offset).Asc("id").Find(&users)
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
	minutes := base.Service.ActiveCodeLives

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
	if err = orm.Find(&accesses, &Access{UserName: user.LowerName}); err != nil {
		return err
	}

	sess := orm.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for i := range accesses {
		accesses[i].UserName = newUserName
		if strings.HasPrefix(accesses[i].RepoName, user.LowerName+"/") {
			accesses[i].RepoName = strings.Replace(accesses[i].RepoName, user.LowerName, newUserName, 1)
			if err = UpdateAccessWithSession(sess, &accesses[i]); err != nil {
				return err
			}
		}
	}

	repos, err := GetRepositories(user, true)
	if err != nil {
		return err
	}
	for i := range repos {
		accesses = make([]Access, 0, 10)
		// Update accesses of user repository.
		if err = orm.Find(&accesses, &Access{RepoName: user.LowerName + "/" + repos[i].LowerName}); err != nil {
			return err
		}

		for j := range accesses {
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
func UpdateUser(user *User) (err error) {
	user.LowerName = strings.ToLower(user.Name)

	if len(user.Location) > 255 {
		user.Location = user.Location[:255]
	}
	if len(user.Website) > 255 {
		user.Website = user.Website[:255]
	}

	_, err = orm.Id(user.Id).AllCols().Update(user)
	return err
}

// DeleteUser completely deletes everything of the user.
func DeleteUser(user *User) error {
	// Check ownership of repository.
	count, err := GetRepositoryCount(user)
	if err != nil {
		return errors.New("modesl.GetRepositories: " + err.Error())
	} else if count > 0 {
		return ErrUserOwnRepos
	}

	// TODO: check issues, other repos' commits

	// Delete all followers.
	if _, err = orm.Delete(&Follow{FollowId: user.Id}); err != nil {
		return err
	}

	// Delete oauth2.
	if _, err = orm.Delete(&Oauth2{Uid: user.Id}); err != nil {
		return err
	}

	// Delete all feeds.
	if _, err = orm.Delete(&Action{UserId: user.Id}); err != nil {
		return err
	}

	// Delete all watches.
	if _, err = orm.Delete(&Watch{UserId: user.Id}); err != nil {
		return err
	}

	// Delete all accesses.
	if _, err = orm.Delete(&Access{UserName: user.LowerName}); err != nil {
		return err
	}

	// Delete all SSH keys.
	keys := make([]PublicKey, 0, 10)
	if err = orm.Find(&keys, &PublicKey{OwnerId: user.Id}); err != nil {
		return err
	}
	for _, key := range keys {
		if err = DeletePublicKey(&key); err != nil {
			return err
		}
	}

	// Delete user directory.
	if err = os.RemoveAll(UserPath(user.Name)); err != nil {
		return err
	}

	_, err = orm.Delete(user)
	return err
}

// UserPath returns the path absolute path of user repositories.
func UserPath(userName string) string {
	return filepath.Join(base.RepoRootPath, strings.ToLower(userName))
}

func GetUserByKeyId(keyId int64) (*User, error) {
	user := new(User)
	rawSql := "SELECT a.* FROM `user` AS a, public_key AS b WHERE a.id = b.owner_id AND b.id=?"
	has, err := orm.Sql(rawSql, keyId).Get(user)
	if err != nil {
		return nil, err
	} else if !has {
		err = errors.New("not exist key owner")
		return nil, err
	}
	return user, nil
}

// GetUserById returns the user object by given id if exists.
func GetUserById(id int64) (*User, error) {
	user := new(User)
	has, err := orm.Id(id).Get(user)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrUserNotExist
	}
	return user, nil
}

// GetUserByName returns the user object by given name if exists.
func GetUserByName(name string) (*User, error) {
	if len(name) == 0 {
		return nil, ErrUserNotExist
	}
	user := &User{LowerName: strings.ToLower(name)}
	has, err := orm.Get(user)
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

// GetUserByEmail returns the user object by given e-mail if exists.
func GetUserByEmail(email string) (*User, error) {
	if len(email) == 0 {
		return nil, ErrUserNotExist
	}
	user := &User{Email: strings.ToLower(email)}
	has, err := orm.Get(user)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist
	}
	return user, nil
}

// LoginUserPlain validates user by raw user name and password.
func LoginUserPlain(name, passwd string) (*User, error) {
	user := User{LowerName: strings.ToLower(name)}
	has, err := orm.Get(&user)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist
	}

	newUser := &User{Passwd: passwd, Salt: user.Salt}
	newUser.EncodePasswd()
	if user.Passwd != newUser.Passwd {
		return nil, ErrUserNotExist
	}
	return &user, nil
}

// Follow is connection request for receiving user notifycation.
type Follow struct {
	Id       int64
	UserId   int64 `xorm:"unique(follow)"`
	FollowId int64 `xorm:"unique(follow)"`
}

// FollowUser marks someone be another's follower.
func FollowUser(userId int64, followId int64) (err error) {
	session := orm.NewSession()
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
	session := orm.NewSession()
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
