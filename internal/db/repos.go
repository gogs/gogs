// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"

	"gogs.io/gogs/internal/errutil"
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

type ErrRepoAlreadyExist struct {
	args errutil.Args
}

func IsErrRepoAlreadyExist(err error) bool {
	_, ok := err.(ErrRepoAlreadyExist)
	return ok
}

func (err ErrRepoAlreadyExist) Error() string {
	return fmt.Sprintf("repository already exists: %v", err.args)
}

// create creates a new repository record in the database. Fields of "repo" will be updated
// in place upon insertion. It returns ErrNameNotAllowed when the repository name is not allowed,
// or returns ErrRepoAlreadyExist when a repository with same name already exists for the owner.
func (db *repos) create(ownerID int64, repo *Repository) error {
	err := isRepoNameAllowed(repo.Name)
	if err != nil {
		return err
	}

	_, err = db.GetByName(ownerID, repo.Name)
	if err == nil {
		return ErrRepoAlreadyExist{args: errutil.Args{"ownerID": ownerID, "name": repo.Name}}
	} else if !gorm.IsRecordNotFoundError(err) {
		return err
	}

	return db.DB.Create(repo).Error
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
