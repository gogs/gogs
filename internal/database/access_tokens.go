// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	gouuid "github.com/satori/go.uuid"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/errutil"
)

// AccessToken is a personal access token.
type AccessToken struct {
	ID     int64 `gorm:"primarykey"`
	UserID int64 `xorm:"uid" gorm:"column:uid;index"`
	Name   string
	Sha1   string `gorm:"type:VARCHAR(40);unique"`
	SHA256 string `gorm:"type:VARCHAR(64);unique;not null"`

	Created           time.Time `gorm:"-" json:"-"`
	CreatedUnix       int64
	Updated           time.Time `gorm:"-" json:"-"`
	UpdatedUnix       int64
	HasRecentActivity bool `gorm:"-" json:"-"`
	HasUsed           bool `gorm:"-" json:"-"`
}

// BeforeCreate implements the GORM create hook.
func (t *AccessToken) BeforeCreate(tx *gorm.DB) error {
	if t.CreatedUnix == 0 {
		t.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (t *AccessToken) AfterFind(tx *gorm.DB) error {
	t.Created = time.Unix(t.CreatedUnix, 0).Local()
	if t.UpdatedUnix > 0 {
		t.Updated = time.Unix(t.UpdatedUnix, 0).Local()
		t.HasUsed = t.Updated.After(t.Created)
		t.HasRecentActivity = t.Updated.Add(7 * 24 * time.Hour).After(tx.NowFunc())
	}
	return nil
}

// AccessTokensStore is the storage layer for access tokens.
type AccessTokensStore struct {
	db *gorm.DB
}

func newAccessTokensStore(db *gorm.DB) *AccessTokensStore {
	return &AccessTokensStore{db: db}
}

type ErrAccessTokenAlreadyExist struct {
	args errutil.Args
}

func IsErrAccessTokenAlreadyExist(err error) bool {
	return errors.As(err, &ErrAccessTokenAlreadyExist{})
}

func (err ErrAccessTokenAlreadyExist) Error() string {
	return fmt.Sprintf("access token already exists: %v", err.args)
}

// Create creates a new access token and persist to database. It returns
// ErrAccessTokenAlreadyExist when an access token with same name already exists
// for the user.
func (s *AccessTokensStore) Create(ctx context.Context, userID int64, name string) (*AccessToken, error) {
	err := s.db.WithContext(ctx).Where("uid = ? AND name = ?", userID, name).First(new(AccessToken)).Error
	if err == nil {
		return nil, ErrAccessTokenAlreadyExist{args: errutil.Args{"userID": userID, "name": name}}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	token := cryptoutil.SHA1(gouuid.NewV4().String())
	sha256 := cryptoutil.SHA256(token)

	accessToken := &AccessToken{
		UserID: userID,
		Name:   name,
		Sha1:   sha256[:40], // To pass the column unique constraint, keep the length of SHA1.
		SHA256: sha256,
	}
	if err = s.db.WithContext(ctx).Create(accessToken).Error; err != nil {
		return nil, err
	}

	// Set back the raw access token value, for the sake of the caller.
	accessToken.Sha1 = token
	return accessToken, nil
}

// DeleteByID deletes the access token by given ID.
//
// 🚨 SECURITY: The "userID" is required to prevent attacker deletes arbitrary
// access token that belongs to another user.
func (s *AccessTokensStore) DeleteByID(ctx context.Context, userID, id int64) error {
	return s.db.WithContext(ctx).Where("id = ? AND uid = ?", id, userID).Delete(new(AccessToken)).Error
}

var _ errutil.NotFound = (*ErrAccessTokenNotExist)(nil)

type ErrAccessTokenNotExist struct {
	args errutil.Args
}

// IsErrAccessTokenNotExist returns true if the underlying error has the type
// ErrAccessTokenNotExist.
func IsErrAccessTokenNotExist(err error) bool {
	return errors.As(errors.Cause(err), &ErrAccessTokenNotExist{})
}

func (err ErrAccessTokenNotExist) Error() string {
	return fmt.Sprintf("access token does not exist: %v", err.args)
}

func (ErrAccessTokenNotExist) NotFound() bool {
	return true
}

// GetBySHA1 returns the access token with given SHA1. It returns
// ErrAccessTokenNotExist when not found.
func (s *AccessTokensStore) GetBySHA1(ctx context.Context, sha1 string) (*AccessToken, error) {
	// No need to waste a query for an empty SHA1.
	if sha1 == "" {
		return nil, ErrAccessTokenNotExist{args: errutil.Args{"sha": sha1}}
	}

	sha256 := cryptoutil.SHA256(sha1)
	token := new(AccessToken)
	err := s.db.WithContext(ctx).Where("sha256 = ?", sha256).First(token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccessTokenNotExist{args: errutil.Args{"sha": sha1}}
	} else if err != nil {
		return nil, err
	}
	return token, nil
}

// List returns all access tokens belongs to given user.
func (s *AccessTokensStore) List(ctx context.Context, userID int64) ([]*AccessToken, error) {
	var tokens []*AccessToken
	return tokens, s.db.WithContext(ctx).Where("uid = ?", userID).Order("id ASC").Find(&tokens).Error
}

// Touch updates the updated time of the given access token to the current time.
func (s *AccessTokensStore) Touch(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).
		Model(new(AccessToken)).
		Where("id = ?", id).
		UpdateColumn("updated_unix", s.db.NowFunc().Unix()).
		Error
}
