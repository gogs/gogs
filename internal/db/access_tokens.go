// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"

	"github.com/jinzhu/gorm"

	"gogs.io/gogs/internal/errutil"
)

// AccessTokensStore is the persistent interface for access tokens.
//
// NOTE: All methods are sorted in alphabetical order.
type AccessTokensStore interface {
	// GetBySHA returns the access token with given SHA1.
	// It returns ErrAccessTokenNotExist when not found.
	GetBySHA(sha string) (*AccessToken, error)
	// Save persists all values of given access token.
	Save(t *AccessToken) error
}

var AccessTokens AccessTokensStore

type accessTokens struct {
	*gorm.DB
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
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAccessTokenNotExist{args: errutil.Args{"sha": sha}}
		}
		return nil, err
	}
	return token, nil
}

func (db *accessTokens) Save(t *AccessToken) error {
	return db.DB.Save(t).Error
}
