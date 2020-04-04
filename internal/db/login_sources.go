// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"github.com/jinzhu/gorm"
)

// LoginSourcesStore is the persistent interface for login sources.
//
// NOTE: All methods are sorted in alphabetical order.
type LoginSourcesStore interface {
	// GetByID returns the login source with given ID.
	// It returns ErrLoginSourceNotExist when not found.
	GetByID(id int64) (*LoginSource, error)
}

var LoginSources LoginSourcesStore

type loginSources struct {
	*gorm.DB
}

func (db *loginSources) GetByID(id int64) (*LoginSource, error) {
	source := new(LoginSource)
	err := db.Where("id = ?", id).First(source).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return localLoginSources.GetLoginSourceByID(id)
		}
		return nil, err
	}
	return source, nil
}
