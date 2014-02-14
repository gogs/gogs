package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dchest/scrypt"
) // user type
const (
	Individual = iota + 1
	Organization
)

// login type
const (
	Plain = iota + 1
	LDAP
)

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
	Avatar        string    `xorm:"varchar(2048) not null"`
	Created       time.Time `xorm:"created"`
	Updated       time.Time `xorm:"updated"`
}

type Follow struct {
	Id       int64
	UserId   int64     `xorm:"unique(s)"`
	FollowId int64     `xorm:"unique(s)"`
	Created  time.Time `xorm:"created"`
}

const (
	OpCreateRepo = iota + 1
	OpDeleteRepo
	OpStarRepo
	OpFollowRepo
	OpCommitRepo
	OpPullRequest
)

type Action struct {
	Id      int64
	UserId  int64
	OpType  int
	RepoId  int64
	Content string
	Created time.Time `xorm:"created"`
}

var (
	ErrUserNotExist = errors.New("User not exist")
)

// user's name should be noncased unique
func IsUserExist(name string) (bool, error) {
	return orm.Get(&User{LowerName: strings.ToLower(name)})
}

func RegisterUser(user *User) error {
	_, err := orm.Insert(user)
	return err
}

func UpdateUser(user *User) error {
	_, err := orm.Id(user.Id).Update(user)
	return err
}

func (user *User) EncodePasswd(pass string) error {
	newPasswd, err := scrypt.Key([]byte(user.Passwd), []byte("!#@FDEWREWR&*("), 16384, 8, 1, 64)
	user.Passwd = fmt.Sprintf("%x", newPasswd)
	return err
}

func LoginUserPlain(name, passwd string) (*User, error) {
	user := User{Name: name}
	err := user.EncodePasswd(passwd)
	if err != nil {
		return nil, err
	}

	has, err := orm.Get(&user)
	if !has {
		err = ErrUserNotExist
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func FollowUser(userId int64, followId int64) error {
	session := orm.NewSession()
	defer session.Close()
	session.Begin()
	_, err := session.Insert(&Follow{UserId: userId, FollowId: followId})
	if err != nil {
		session.Rollback()
		return err
	}
	_, err = session.Exec("update user set num_followers = num_followers + 1 where id = ?", followId)
	if err != nil {
		session.Rollback()
		return err
	}
	_, err = session.Exec("update user set num_followings = num_followings + 1 where id = ?", userId)
	if err != nil {
		session.Rollback()
		return err
	}
	return session.Commit()
}

func UnFollowUser(userId int64, unFollowId int64) error {
	session := orm.NewSession()
	defer session.Close()
	session.Begin()
	_, err := session.Delete(&Follow{UserId: userId, FollowId: unFollowId})
	if err != nil {
		session.Rollback()
		return err
	}
	_, err = session.Exec("update user set num_followers = num_followers - 1 where id = ?", unFollowId)
	if err != nil {
		session.Rollback()
		return err
	}
	_, err = session.Exec("update user set num_followings = num_followings - 1 where id = ?", userId)
	if err != nil {
		session.Rollback()
		return err
	}
	return session.Commit()
}
