// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"time"

	gouuid "github.com/satori/go.uuid"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/errutil"
)

// AccessTokensStore is the persistent interface for access tokens.
//
// NOTE: All methods are sorted in alphabetical order.
type AccessTokensStore interface {
	// Create creates a new access token and persist to database.
	// It returns ErrAccessTokenAlreadyExist when an access token
	// with same name already exists for the user.
	Create(ctx context.Context, userID int64, name string) (*AccessToken, error)
	// DeleteByID deletes the access token by given ID.
	// 🚨 SECURITY: The "userID" is required to prevent attacker
	// deletes arbitrary access token that belongs to another user.
	DeleteByID(ctx context.Context, userID, id int64) error
	// GetBySHA returns the access token with given SHA1.
	// It returns ErrAccessTokenNotExist when not found.
	GetBySHA(ctx context.Context, sha string) (*AccessToken, error)
	// List returns all access tokens belongs to given user.
	List(ctx context.Context, userID int64) ([]*AccessToken, error)
	// Save persists all values of given access token.
	// The Updated field is set to current time automatically.
	Save(ctx context.Context, t *AccessToken) error
}

var AccessTokens AccessTokensStore

var _ AccessTokensStore = (*accessTokens)(nil)

type accessTokens struct {
	*gorm.DB
}

// NewAccessTokensStore returns a persistent interface for access tokens with
// given database connection.
func NewAccessTokensStore(db *gorm.DB) AccessTokensStore {
	return &accessTokens{DB: db}
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

func (db *accessTokens) Create(ctx context.Context, userID int64, name string) (*AccessToken, error) {
	err := db.WithContext(ctx).
		Where("uid = ? AND name = ?", userID, name).
		First(new(AccessToken)).Error
	if err == nil {
		return nil, ErrAccessTokenAlreadyExist{args: errutil.Args{"userID": userID, "name": name}}
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	token := &AccessToken{
		UserID: userID,
		Name:   name,
		Sha1:   cryptoutil.SHA1(gouuid.NewV4().String()),
	}
	return token, db.DB.Create(token).Error
}

func (db *accessTokens) DeleteByID(ctx context.Context, userID, id int64) error {
	return db.WithContext(ctx).Where("id = ? AND uid = ?", id, userID).Delete(new(AccessToken)).Error
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

func (db *accessTokens) GetBySHA(ctx context.Context, sha string) (*AccessToken, error) {
	token := new(AccessToken)
	err := db.WithContext(ctx).Where("sha1 = ?", sha).First(token).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAccessTokenNotExist{args: errutil.Args{"sha": sha}}
		}
		return nil, err
	}
	return token, nil
}

func (db *accessTokens) List(ctx context.Context, userID int64) ([]*AccessToken, error) {
	var tokens []*AccessToken
	return tokens, db.WithContext(ctx).Where("uid = ?", userID).Find(&tokens).Error
}

func (db *accessTokens) Save(ctx context.Context, t *AccessToken) error {
	return db.WithContext(ctx).Save(t).Error
}

// AccessToken is a personal access token.
type AccessToken struct {
	ID     int64 `gorm:"primarykey"`
	UserID int64 `xorm:"uid INDEX" gorm:"column:uid;index"`
	Name   string
	Sha1   string `xorm:"UNIQUE VARCHAR(40)" gorm:"type:varchar(40);unique"`

	Created           time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix       int64
	Updated           time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix       int64
	HasRecentActivity bool `xorm:"-" gorm:"-" json:"-"`
	HasUsed           bool `xorm:"-" gorm:"-" json:"-"`
}

// NOTE: This is a GORM create hook.
func (t *AccessToken) BeforeCreate(tx *gorm.DB) error {
	if t.CreatedUnix == 0 {
		t.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// NOTE: This is a GORM update hook.
func (t *AccessToken) BeforeUpdate(tx *gorm.DB) error {
	t.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// NOTE: This is a GORM query hook.
func (t *AccessToken) AfterFind(tx *gorm.DB) error {
	t.Created = time.Unix(t.CreatedUnix, 0).Local()
	if t.UpdatedUnix > 0 {
		t.Updated = time.Unix(t.UpdatedUnix, 0).Local()
		t.HasUsed = t.Updated.After(t.Created)
		t.HasRecentActivity = t.Updated.Add(7 * 24 * time.Hour).After(tx.NowFunc())
	}
	return nil
}
