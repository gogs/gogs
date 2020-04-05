// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"strings"

	"github.com/jinzhu/gorm"
)

// ReposStore is the persistent interface for repositories.
//
// NOTE: All methods are sorted in alphabetical order.
type ReposStore interface {
	// GetByName returns the repository with given owner and name.
	// It returns ErrRepoNotExist when not found.
	GetByName(ownerID int64, name string) (*Repository, error)
}

var Repos ReposStore

type repos struct {
	*gorm.DB
}

func (db *repos) GetByName(ownerID int64, name string) (*Repository, error) {
	repo := new(Repository)
	err := db.Where("owner_id = ? AND lower_name = ?", ownerID, strings.ToLower(name)).First(repo).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrRepoNotExist{args: map[string]interface{}{"ownerID": ownerID, "name": name}}
		}
		return nil, err
	}
	return repo, nil
}
