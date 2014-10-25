// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Unknwon/com"

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
	LowerName     string `xorm:"UNIQUE NOT NULL"`
	Name          string `xorm:"UNIQUE NOT NULL"`
	FullName      string
	Email         string `xorm:"UNIQUE NOT NULL"`
	Passwd        string `xorm:"NOT NULL"`
	LoginType     LoginType
	LoginSource   int64 `xorm:"NOT NULL DEFAULT 0"`
	LoginName     string
	Type          UserType
	Orgs          []*User       `xorm:"-"`
	Repos         []*Repository `xorm:"-"`
	NumFollowers  int
	NumFollowings int
	NumStars      int
	NumRepos      int
	Avatar        string `xorm:"VARCHAR(2048) NOT NULL"`
	AvatarEmail   string `xorm:"NOT NULL"`
	Location      string
	Website       string
	IsActive      bool
	IsAdmin       bool
	Rands         string    `xorm:"VARCHAR(10)"`
	Salt          string    `xorm:"VARCHAR(10)"`
	Created       time.Time `xorm:"CREATED"`
	Updated       time.Time `xorm:"UPDATED"`

	// For organization.
	Description string
	NumTeams    int
	NumMembers  int
	Teams       []*Team `xorm:"-"`
	Members     []*User `xorm:"-"`
}

// DashboardLink returns the user dashboard page link.
func (u *User) DashboardLink() string {
	if u.IsOrganization() {
		return setting.AppSubUrl + "/org/" + u.Name + "/dashboard/"
	}
	return setting.AppSubUrl + "/"
}

// HomeLink returns the user home page link.
func (u *User) HomeLink() string {
	return setting.AppSubUrl + "/" + u.Name
}

// AvatarLink returns user gravatar link.
func (u *User) AvatarLink() string {
	if setting.DisableGravatar {
		return setting.AppSubUrl + "/img/avatar_default.jpg"
	} else if setting.Service.EnableCacheAvatar {
		return setting.AppSubUrl + "/avatar/" + u.Avatar
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

// ValidtePassword checks if given password matches the one belongs to the user.
func (u *User) ValidtePassword(passwd string) bool {
	newUser := &User{Passwd: passwd, Salt: u.Salt}
	newUser.EncodePasswd()
	return u.Passwd == newUser.Passwd
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
		u.Orgs[i], err = GetUserById(ou.OrgId)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetFullNameFallback returns Full Name if set, otherwise username
func (u *User) GetFullNameFallback() string {
	if u.FullName == "" {
		return u.Name
	}
	return u.FullName
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

// GetUserSalt returns a ramdom user salt token.
func GetUserSalt() string {
	return base.GetRandomString(10)
}

// CreateUser creates record of a new user.
func CreateUser(u *User) error {
	if !IsLegalName(u.Name) {
		return ErrUserNameIllegal
	}

	isExist, err := IsUserExist(u.Name)
	if err != nil {
		return err
	} else if isExist {
		return ErrUserAlreadyExist
	}

	isExist, err = IsEmailUsed(u.Email)
	if err != nil {
		return err
	} else if isExist {
		return ErrEmailAlreadyUsed
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
		return err
	}

	if _, err = sess.Insert(u); err != nil {
		sess.Rollback()
		return err
	} else if err = os.MkdirAll(UserPath(u.Name), os.ModePerm); err != nil {
		sess.Rollback()
		return err
	} else if err = sess.Commit(); err != nil {
		return err
	}

	// Auto-set admin for user whose ID is 1.
	if u.Id == 1 {
		u.IsAdmin = true
		u.IsActive = true
		_, err = x.Id(u.Id).UseBool().Update(u)
	}
	return err
}

// CountUsers returns number of users.
func CountUsers() int64 {
	count, _ := x.Where("type=0").Count(new(User))
	return count
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

// ChangeUserName changes all corresponding setting from old user name to new one.
func ChangeUserName(u *User, newUserName string) (err error) {
	if !IsLegalName(newUserName) {
		return ErrUserNameIllegal
	}

	newUserName = strings.ToLower(newUserName)

	// Update accesses of user.
	accesses := make([]Access, 0, 10)
	if err = x.Find(&accesses, &Access{UserName: u.LowerName}); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for i := range accesses {
		accesses[i].UserName = newUserName
		if strings.HasPrefix(accesses[i].RepoName, u.LowerName+"/") {
			accesses[i].RepoName = strings.Replace(accesses[i].RepoName, u.LowerName, newUserName, 1)
		}
		if err = UpdateAccessWithSession(sess, &accesses[i]); err != nil {
			return err
		}
	}

	repos, err := GetRepositories(u.Id, true)
	if err != nil {
		return err
	}
	for i := range repos {
		accesses = make([]Access, 0, 10)
		// Update accesses of user repository.
		if err = x.Find(&accesses, &Access{RepoName: u.LowerName + "/" + repos[i].LowerName}); err != nil {
			return err
		}

		for j := range accesses {
			// if the access is not the user's access (already updated above)
			if accesses[j].UserName != u.LowerName {
				accesses[j].RepoName = newUserName + "/" + repos[i].LowerName
				if err = UpdateAccessWithSession(sess, &accesses[j]); err != nil {
					return err
				}
			}
		}
	}

	// Change user directory name.
	if err = os.Rename(UserPath(u.LowerName), UserPath(newUserName)); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

// UpdateUser updates user's information.
func UpdateUser(u *User) error {
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

	_, err := x.Id(u.Id).AllCols().Update(u)
	return err
}

// TODO: need some kind of mechanism to record failure.
// DeleteUser completely and permanently deletes everything of user.
func DeleteUser(u *User) error {
	// Check ownership of repository.
	count, err := GetRepositoryCount(u)
	if err != nil {
		return errors.New("GetRepositoryCount: " + err.Error())
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

// GetUserByName returns user by given name.
func GetUserByName(name string) (*User, error) {
	if len(name) == 0 {
		return nil, ErrUserNotExist
	}
	u := &User{LowerName: strings.ToLower(name)}
	has, err := x.Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist
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

// UserCommit represtns a commit with validation of user.
type UserCommit struct {
	UserName string
	*git.Commit
}

// ValidateCommitWithEmail chceck if author's e-mail of commit is corresponsind to a user.
func ValidateCommitWithEmail(c *git.Commit) (uname string) {
	u, err := GetUserByEmail(c.Author.Email)
	if err == nil {
		uname = u.Name
	}
	return uname
}

// ValidateCommitsWithEmails checks if authors' e-mails of commits are corresponding to users.
func ValidateCommitsWithEmails(oldCommits *list.List) *list.List {
	emails := map[string]string{}
	newCommits := list.New()
	e := oldCommits.Front()
	for e != nil {
		c := e.Value.(*git.Commit)

		uname := ""
		if v, ok := emails[c.Author.Email]; !ok {
			u, err := GetUserByEmail(c.Author.Email)
			if err == nil {
				uname = u.Name
			}
			emails[c.Author.Email] = uname
		} else {
			uname = v
		}

		newCommits.PushBack(UserCommit{
			UserName: uname,
			Commit:   c,
		})
		e = e.Next()
	}
	return newCommits
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
func SearchUserByName(opt SearchOption) (us []*User, err error) {
	opt.Keyword = FilterSQLInject(opt.Keyword)
	if len(opt.Keyword) == 0 {
		return us, nil
	}
	opt.Keyword = strings.ToLower(opt.Keyword)

	us = make([]*User, 0, opt.Limit)
	err = x.Limit(opt.Limit).Where("type=0").And("lower_name like '%" + opt.Keyword + "%'").Find(&us)
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
	sess := x.NewSession()
	defer sess.Close()
	sess.Begin()

	if _, err = sess.Insert(&Follow{UserId: userId, FollowId: followId}); err != nil {
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

func UpdateMentions(userNames []string, issueId int64) error {
	users := make([]*User, 0, len(userNames))

	if err := x.Where("name IN (?)", strings.Join(userNames, "\",\"")).OrderBy("name ASC").Find(&users); err != nil {
		return err
	}

	ids := make([]int64, 0, len(userNames))

	for _, user := range users {
		ids = append(ids, user.Id)

		if user.Type == INDIVIDUAL {
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
			tempIds = append(tempIds, orgUser.Id)
		}

		ids = append(ids, tempIds...)
	}

	if err := UpdateIssueUserPairsByMentions(ids, issueId); err != nil {
		return err
	}

	return nil
}
