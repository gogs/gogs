// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dchest/scrypt"

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
	Created       time.Time `xorm:"created"`
	Updated       time.Time `xorm:"updated"`
}

// HomeLink returns the user home page link.
func (user *User) HomeLink() string {
	return "/user/" + user.LowerName
}

// AvatarLink returns the user gravatar link.
func (user *User) AvatarLink() string {
	return "http://1.gravatar.com/avatar/" + user.Avatar
}

type Follow struct {
	Id       int64
	UserId   int64     `xorm:"unique(s)"`
	FollowId int64     `xorm:"unique(s)"`
	Created  time.Time `xorm:"created"`
}

var (
	ErrUserOwnRepos     = errors.New("User still have ownership of repositories")
	ErrUserAlreadyExist = errors.New("User already exist")
	ErrUserNotExist     = errors.New("User does not exist")
	ErrEmailAlreadyUsed = errors.New("E-mail already used")
)

// IsUserExist checks if given user name exist,
// the user name should be noncased unique.
func IsUserExist(name string) (bool, error) {
	return orm.Get(&User{LowerName: strings.ToLower(name)})
}

// IsEmailUsed returns true if the e-mail has been used.
func IsEmailUsed(email string) (bool, error) {
	return orm.Get(&User{Email: email})
}

// NewGitSig generates and returns the signature of given user.
func (user *User) NewGitSig() *git.Signature {
	return &git.Signature{
		Name:  user.Name,
		Email: user.Email,
		When:  time.Now(),
	}
}

// return a user salt token
func GetUserSalt() string {
	return base.GetRandomString(10)
}

// RegisterUser creates record of a new user.
func RegisterUser(user *User) (*User, error) {
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
	if err = user.EncodePasswd(); err != nil {
		return nil, err
	} else if _, err = orm.Insert(user); err != nil {
		return nil, err
	} else if err = os.MkdirAll(UserPath(user.Name), os.ModePerm); err != nil {
		if _, err := orm.Id(user.Id).Delete(&User{}); err != nil {
			return nil, errors.New(fmt.Sprintf(
				"both create userpath %s and delete table record faild: %v", user.Name, err))
		}
		return nil, err
	}
	return user, nil
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

// UpdateUser updates user's information.
func UpdateUser(user *User) (err error) {
	_, err = orm.Id(user.Id).UseBool().Update(user)
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

	// Delete all feeds.
	if _, err = orm.Delete(&Action{UserId: user.Id}); err != nil {
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
	// TODO: delete and update follower information.
	return err
}

// EncodePasswd encodes password to safe format.
func (user *User) EncodePasswd() error {
	newPasswd, err := scrypt.Key([]byte(user.Passwd), []byte(base.SecretKey), 16384, 8, 1, 64)
	user.Passwd = fmt.Sprintf("%x", newPasswd)
	return err
}

// UserPath returns the path absolute path of user repositories.
func UserPath(userName string) string {
	return filepath.Join(RepoRootPath, strings.ToLower(userName))
}

func GetUserByKeyId(keyId int64) (*User, error) {
	user := new(User)
	rawSql := "SELECT a.* FROM user AS a, public_key AS b WHERE a.id = b.owner_id AND b.id=?"
	if base.Cfg.MustValue("database", "DB_TYPE") == "postgres" {
		rawSql = "SELECT a.* FROM \"user\" AS a, public_key AS b WHERE a.id = b.owner_id AND b.id=?"
	}
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
	user := &User{
		LowerName: strings.ToLower(name),
	}
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
	user := User{LowerName: strings.ToLower(name), Passwd: passwd}
	if err := user.EncodePasswd(); err != nil {
		return nil, err
	}

	has, err := orm.Get(&user)
	if err != nil {
		return nil, err
	} else if !has {
		err = ErrUserNotExist
	}
	return &user, err
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

	rawSql := "UPDATE user SET num_followers = num_followers + 1 WHERE id = ?"
	if base.Cfg.MustValue("database", "DB_TYPE") == "postgres" {
		rawSql = "UPDATE \"user\" SET num_followers = num_followers + 1 WHERE id = ?"
	}
	if _, err = session.Exec(rawSql, followId); err != nil {
		session.Rollback()
		return err
	}

	rawSql = "UPDATE user SET num_followings = num_followings + 1 WHERE id = ?"
	if base.Cfg.MustValue("database", "DB_TYPE") == "postgres" {
		rawSql = "UPDATE \"user\" SET num_followings = num_followings + 1 WHERE id = ?"
	}
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

	rawSql := "UPDATE user SET num_followers = num_followers - 1 WHERE id = ?"
	if base.Cfg.MustValue("database", "DB_TYPE") == "postgres" {
		rawSql = "UPDATE \"user\" SET num_followers = num_followers - 1 WHERE id = ?"
	}
	if _, err = session.Exec(rawSql, unFollowId); err != nil {
		session.Rollback()
		return err
	}

	rawSql = "UPDATE user SET num_followings = num_followings - 1 WHERE id = ?"
	if base.Cfg.MustValue("database", "DB_TYPE") == "postgres" {
		rawSql = "UPDATE \"user\" SET num_followings = num_followings - 1 WHERE id = ?"
	}
	if _, err = session.Exec(rawSql, userId); err != nil {
		session.Rollback()
		return err
	}
	return session.Commit()
}
