// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	_ "image/jpeg"
	"os"
	"strings"
	"time"

	log "unknwon.dev/clog/v2"
	"xorm.io/xorm"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/repoutil"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userutil"
)

// TODO(unknwon): Delete me once refactoring is done.
func (u *User) BeforeInsert() {
	u.CreatedUnix = time.Now().Unix()
	u.UpdatedUnix = u.CreatedUnix
}

// TODO(unknwon): Delete me once refactoring is done.
func (u *User) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		u.Created = time.Unix(u.CreatedUnix, 0).Local()
	case "updated_unix":
		u.Updated = time.Unix(u.UpdatedUnix, 0).Local()
	}
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

		if u.AvatarEmail == "" {
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

// TODO(unknwon): Refactoring together with methods that do updates.
func (u *User) BeforeUpdate() {
	if u.MaxRepoCreation < -1 {
		u.MaxRepoCreation = -1
	}
	u.UpdatedUnix = time.Now().Unix()
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
		&EmailAddress{UserID: u.ID},
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

	_ = os.RemoveAll(repoutil.UserPath(u.Name))
	_ = os.Remove(userutil.CustomAvatarPath(u.ID))

	return nil
}

// Deprecated: Use OrgsUsers.CountByUser instead.
//
// TODO(unknwon): Delete me once no more call sites in this file.
func (u *User) getOrganizationCount(e Engine) (int64, error) {
	return e.Where("uid=?", u.ID).Count(new(OrgUser))
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

// GetAssigneeByID returns the user with read access of repository by given ID.
func GetAssigneeByID(repo *Repository, userID int64) (*User, error) {
	ctx := context.TODO()
	if !Perms.Authorize(ctx, userID, repo.ID, AccessModeRead,
		AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		},
	) {
		return nil, ErrUserNotExist{args: map[string]interface{}{"userID": userID}}
	}
	return Users.GetByID(ctx, userID)
}

// GetUserEmailsByNames returns a list of e-mails corresponds to names.
func GetUserEmailsByNames(names []string) []string {
	mails := make([]string, 0, len(names))
	for _, name := range names {
		u, err := Users.GetByUsername(context.TODO(), name)
		if err != nil {
			continue
		}
		if u.IsMailable() {
			mails = append(mails, u.Email)
		}
	}
	return mails
}

// UserCommit represents a commit with validation of user.
type UserCommit struct {
	User *User
	*git.Commit
}

// ValidateCommitWithEmail checks if author's e-mail of commit is corresponding to a user.
func ValidateCommitWithEmail(c *git.Commit) *User {
	u, err := Users.GetByEmail(context.TODO(), c.Author.Email)
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
			u, _ = Users.GetByEmail(context.TODO(), oldCommits[i].Author.Email)
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
	if opts.Keyword == "" {
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
