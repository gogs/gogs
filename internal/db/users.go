// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"

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
	// GetByID returns the user with given ID. It returns ErrUserNotExist when not found.
	GetByID(id int64) (*User, error)
	// GetByUsername returns the user with given username. It returns ErrUserNotExist
	// when not found.
	GetByUsername(username string) (*User, error)
}

var Users UsersStore

type users struct {
	*gorm.DB
}

type ErrLoginSourceMismatch struct {
	args errutil.Args
}

func (err ErrLoginSourceMismatch) Error() string {
	return fmt.Sprintf("login source mismatch: %v", err.args)
}

func (db *users) Authenticate(username, password string, loginSourceID int64) (*User, error) {
	username = strings.ToLower(username)

	var query *gorm.DB
	if strings.Contains(username, "@") {
		query = db.Where("email = ?", username)
	} else {
		query = db.Where("lower_name = ?", username)
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

		_, err = authenticateViaLoginSource(source, username, password, false)
		if err != nil {
			return nil, errors.Wrap(err, "authenticate via login source")
		}
		return user, nil
	}

	// Non-local login source is always greater than 0.
	if loginSourceID <= 0 {
		return nil, ErrUserNotExist{args: map[string]interface{}{"name": username}}
	}

	source, err := LoginSources.GetByID(loginSourceID)
	if err != nil {
		return nil, errors.Wrap(err, "get login source")
	}

	user, err = authenticateViaLoginSource(source, username, password, true)
	if err != nil {
		return nil, errors.Wrap(err, "authenticate via login source")
	}
	return user, nil
}

func (db *users) GetByID(id int64) (*User, error) {
	user := new(User)
	err := db.Where("id = ?", id).First(user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotExist{args: map[string]interface{}{"userID": id}}
		}
		return nil, err
	}
	return user, nil
}

func (db *users) GetByUsername(username string) (*User, error) {
	user := new(User)
	err := db.Where("lower_name = ?", strings.ToLower(username)).First(user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotExist{args: map[string]interface{}{"name": username}}
		}
		return nil, err
	}
	return user, nil
}
