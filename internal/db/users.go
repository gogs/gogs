// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/errutil"
)

// UsersStore is the persistent interface for users.
//
// NOTE: All methods are sorted in alphabetical order.
type UsersStore interface {
	// Authenticate validates username and password via given login source ID.
	// It returns ErrUserNotExist when the user was not found.
	//
	// When the "loginSourceID" is negative, it aborts the process and returns
	// ErrUserNotExist if the user was not found in the database.
	//
	// When the "loginSourceID" is non-negative, it returns ErrLoginSourceMismatch
	// if the user has different login source ID than the "loginSourceID".
	//
	// When the "loginSourceID" is positive, it tries to authenticate via given
	// login source and creates a new user when not yet exists in the database.
	Authenticate(username, password string, loginSourceID int64) (*User, error)
	// Create creates a new user and persist to database.
	// It returns ErrUserAlreadyExist when a user with same name already exists,
	// or ErrEmailAlreadyUsed if the email has been used by another user.
	Create(opts CreateUserOpts) (*User, error)
	// GetByEmail returns the user (not organization) with given email.
	// It ignores records with unverified emails and returns ErrUserNotExist when not found.
	GetByEmail(email string) (*User, error)
	// GetByID returns the user with given ID. It returns ErrUserNotExist when not found.
	GetByID(id int64) (*User, error)
	// GetByUsername returns the user with given username. It returns ErrUserNotExist when not found.
	GetByUsername(username string) (*User, error)
}

var Users UsersStore

// NOTE: This is a GORM create hook.
func (u *User) BeforeCreate() {
	u.CreatedUnix = gorm.NowFunc().Unix()
	u.UpdatedUnix = u.CreatedUnix
}

// NOTE: This is a GORM query hook.
func (u *User) AfterFind() {
	u.Created = time.Unix(u.CreatedUnix, 0).Local()
	u.Updated = time.Unix(u.UpdatedUnix, 0).Local()
}

var _ UsersStore = (*users)(nil)

type users struct {
	*gorm.DB
}

type ErrLoginSourceMismatch struct {
	args errutil.Args
}

func (err ErrLoginSourceMismatch) Error() string {
	return fmt.Sprintf("login source mismatch: %v", err.args)
}

func (db *users) Authenticate(login, password string, loginSourceID int64) (*User, error) {
	login = strings.ToLower(login)

	var query *gorm.DB
	if strings.Contains(login, "@") {
		query = db.Where("email = ?", login)
	} else {
		query = db.Where("lower_name = ?", login)
	}

	user := new(User)
	err := query.First(user).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, errors.Wrap(err, "get user")
	}

	// User found in the database
	if err == nil {
		// Note: This check is unnecessary but to reduce user confusion at login page
		// and make it more consistent from user's perspective.
		if loginSourceID >= 0 && user.LoginSource != loginSourceID {
			return nil, ErrLoginSourceMismatch{args: errutil.Args{"expect": loginSourceID, "actual": user.LoginSource}}
		}

		// Validate password hash fetched from database for local accounts.
		if user.LoginType == LoginNotype || user.LoginType == LoginPlain {
			if user.ValidatePassword(password) {
				return user, nil
			}

			return nil, ErrUserNotExist{args: map[string]interface{}{"userID": user.ID, "name": user.Name}}
		}

		source, err := LoginSources.GetByID(user.LoginSource)
		if err != nil {
			return nil, errors.Wrap(err, "get login source")
		}

		_, err = authenticateViaLoginSource(source, login, password, false)
		if err != nil {
			return nil, errors.Wrap(err, "authenticate via login source")
		}
		return user, nil
	}

	// Non-local login source is always greater than 0.
	if loginSourceID <= 0 {
		return nil, ErrUserNotExist{args: map[string]interface{}{"login": login}}
	}

	source, err := LoginSources.GetByID(loginSourceID)
	if err != nil {
		return nil, errors.Wrap(err, "get login source")
	}

	user, err = authenticateViaLoginSource(source, login, password, true)
	if err != nil {
		return nil, errors.Wrap(err, "authenticate via login source")
	}
	return user, nil
}

type CreateUserOpts struct {
	Name        string
	Email       string
	Password    string
	LoginSource int64
	Activated   bool
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

func (db *users) Create(opts CreateUserOpts) (*User, error) {
	err := isUsernameAllowed(opts.Name)
	if err != nil {
		return nil, err
	}

	_, err = db.GetByUsername(opts.Name)
	if err == nil {
		return nil, ErrUserAlreadyExist{args: errutil.Args{"name": opts.Name}}
	} else if !IsErrUserNotExist(err) {
		return nil, err
	}

	_, err = db.GetByEmail(opts.Email)
	if err == nil {
		return nil, ErrEmailAlreadyUsed{args: errutil.Args{"email": opts.Email}}
	} else if !IsErrUserNotExist(err) {
		return nil, err
	}

	user := &User{
		LowerName:       strings.ToLower(opts.Name),
		Name:            opts.Name,
		Email:           opts.Email,
		Passwd:          opts.Password,
		LoginSource:     opts.LoginSource,
		MaxRepoCreation: -1,
		IsActive:        opts.Activated,
		Avatar:          cryptoutil.MD5(opts.Email),
		AvatarEmail:     opts.Email,
	}

	user.Rands, err = GetUserSalt()
	if err != nil {
		return nil, err
	}
	user.Salt, err = GetUserSalt()
	if err != nil {
		return nil, err
	}
	user.EncodePassword()

	return user, db.DB.Create(user).Error
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

func (db *users) GetByEmail(email string) (*User, error) {
	email = strings.ToLower(email)

	if len(email) == 0 {
		return nil, ErrUserNotExist{args: errutil.Args{"email": email}}
	}

	// First try to find the user by primary email
	user := new(User)
	err := db.Where("email = ? AND type = ? AND is_active = ?", email, UserIndividual, true).First(user).Error
	if err == nil {
		return user, nil
	} else if !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}

	// Otherwise, check activated email addresses
	emailAddress := new(EmailAddress)
	err = db.Where("email = ? AND is_activated = ?", email, true).First(emailAddress).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrUserNotExist{args: errutil.Args{"email": email}}
		}
		return nil, err
	}

	return db.GetByID(emailAddress.UID)
}

func (db *users) GetByID(id int64) (*User, error) {
	user := new(User)
	err := db.Where("id = ?", id).First(user).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrUserNotExist{args: errutil.Args{"userID": id}}
		}
		return nil, err
	}
	return user, nil
}

func (db *users) GetByUsername(username string) (*User, error) {
	user := new(User)
	err := db.Where("lower_name = ?", strings.ToLower(username)).First(user).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrUserNotExist{args: errutil.Args{"name": username}}
		}
		return nil, err
	}
	return user, nil
}
