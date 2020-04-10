// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	gouuid "github.com/satori/go.uuid"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/tool"
)

// AccessTokensStore is the persistent interface for access tokens.
//
// NOTE: All methods are sorted in alphabetical order.
type AccessTokensStore interface {
	// Create creates a new access token and persist to database.
	// It returns ErrAccessTokenAlreadyExist when an access token
	// with same name already exists for the user.
	Create(userID int64, name string) (*AccessToken, error)
	// DeleteByID deletes the access token by given ID.
	// 🚨 SECURITY: The "userID" is required to prevent attacker
	// deletes arbitrary access token that belongs to another user.
	DeleteByID(userID, id int64) error
	// GetBySHA returns the access token with given SHA1.
	// It returns ErrAccessTokenNotExist when not found.
	GetBySHA(sha string) (*AccessToken, error)
	// List returns all access tokens belongs to given user.
	List(userID int64) ([]*AccessToken, error)
	// Save persists all values of given access token.
	// The Updated field is set to current time automatically.
	Save(t *AccessToken) error
}

var AccessTokens AccessTokensStore

// AccessToken is a personal access token.
type AccessToken struct {
	ID     int64
	UserID int64 `xorm:"uid INDEX" gorm:"COLUMN:uid;INDEX"`
	Name   string
	Sha1   string `xorm:"UNIQUE VARCHAR(40)" gorm:"TYPE:VARCHAR(40);UNIQUE"`

	Created           time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix       int64
	Updated           time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix       int64
	HasRecentActivity bool `xorm:"-" gorm:"-" json:"-"`
	HasUsed           bool `xorm:"-" gorm:"-" json:"-"`
}

// NOTE: This is a GORM create hook.
func (t *AccessToken) BeforeCreate() {
	t.CreatedUnix = t.Created.Unix()
}

// NOTE: This is a GORM update hook.
func (t *AccessToken) BeforeUpdate() {
	t.UpdatedUnix = t.Updated.Unix()
}

// NOTE: This is a GORM query hook.
func (t *AccessToken) AfterFind() {
	t.Created = time.Unix(t.CreatedUnix, 0).Local()
	t.Updated = time.Unix(t.UpdatedUnix, 0).Local()
	t.HasUsed = t.Updated.After(t.Created)
	t.HasRecentActivity = t.Updated.Add(7 * 24 * time.Hour).After(time.Now())
}

var _ AccessTokensStore = (*accessTokens)(nil)

type accessTokens struct {
	*gorm.DB
	clock func() time.Time
}

type ErrAccessTokenAlreadyExist struct {
	args errutil.Args
}

func IsErrAccessTokenAlreadyExist(err error) bool {
	_, ok := err.(ErrAccessTokenAlreadyExist)
	return ok
}

func (err ErrAccessTokenAlreadyExist) Error() string {
	return fmt.Sprintf("access token already exists: %v", err.args)
}

func (db *accessTokens) Create(userID int64, name string) (*AccessToken, error) {
	err := db.Where("uid = ? AND name = ?", userID, name).First(new(AccessToken)).Error
	if err == nil {
		return nil, ErrAccessTokenAlreadyExist{args: errutil.Args{"userID": userID, "name": name}}
	} else if !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}

	token := &AccessToken{
		UserID:  userID,
		Name:    name,
		Sha1:    tool.SHA1(gouuid.NewV4().String()),
		Created: db.clock(),
	}
	return token, db.DB.Create(token).Error
}

func (db *accessTokens) DeleteByID(userID, id int64) error {
	return db.Where("id = ? AND uid = ?", id, userID).Delete(new(AccessToken)).Error
}

var _ errutil.NotFound = (*ErrAccessTokenNotExist)(nil)

type ErrAccessTokenNotExist struct {
	args errutil.Args
}

func IsErrAccessTokenNotExist(err error) bool {
	_, ok := err.(ErrAccessTokenNotExist)
	return ok
}

func (err ErrAccessTokenNotExist) Error() string {
	return fmt.Sprintf("access token does not exist: %v", err.args)
}

func (ErrAccessTokenNotExist) NotFound() bool {
	return true
}

func (db *accessTokens) GetBySHA(sha string) (*AccessToken, error) {
	token := new(AccessToken)
	err := db.Where("sha1 = ?", sha).First(token).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrAccessTokenNotExist{args: errutil.Args{"sha": sha}}
		}
		return nil, err
	}
	return token, nil
}

func (db *accessTokens) List(userID int64) ([]*AccessToken, error) {
	var tokens []*AccessToken
	return tokens, db.Where("uid = ?", userID).Find(&tokens).Error
}

func (db *accessTokens) Save(t *AccessToken) error {
	t.Updated = db.clock()
	return db.DB.Save(t).Error
}
