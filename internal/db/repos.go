// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

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

// NOTE: This is a GORM create hook.
func (r *Repository) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedUnix == 0 {
		r.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// NOTE: This is a GORM update hook.
func (r *Repository) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// NOTE: This is a GORM query hook.
func (r *Repository) AfterFind(tx *gorm.DB) error {
	r.Created = time.Unix(r.CreatedUnix, 0).Local()
	r.Updated = time.Unix(r.UpdatedUnix, 0).Local()
	return nil
}

var _ ReposStore = (*repos)(nil)

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

type createRepoOpts struct {
	Name          string
	Description   string
	DefaultBranch string
	Private       bool
	Mirror        bool
	EnableWiki    bool
	EnableIssues  bool
	EnablePulls   bool
	Fork          bool
	ForkID        int64
}

// create creates a new repository record in the database. Fields of "repo" will be updated
// in place upon insertion. It returns ErrNameNotAllowed when the repository name is not allowed,
// or ErrRepoAlreadyExist when a repository with same name already exists for the owner.
func (db *repos) create(ownerID int64, opts createRepoOpts) (*Repository, error) {
	err := isRepoNameAllowed(opts.Name)
	if err != nil {
		return nil, err
	}

	_, err = db.GetByName(ownerID, opts.Name)
	if err == nil {
		return nil, ErrRepoAlreadyExist{args: errutil.Args{"ownerID": ownerID, "name": opts.Name}}
	} else if !IsErrRepoNotExist(err) {
		return nil, err
	}

	repo := &Repository{
		OwnerID:       ownerID,
		LowerName:     strings.ToLower(opts.Name),
		Name:          opts.Name,
		Description:   opts.Description,
		DefaultBranch: opts.DefaultBranch,
		IsPrivate:     opts.Private,
		IsMirror:      opts.Mirror,
		EnableWiki:    opts.EnableWiki,
		EnableIssues:  opts.EnableIssues,
		EnablePulls:   opts.EnablePulls,
		IsFork:        opts.Fork,
		ForkID:        opts.ForkID,
	}
	return repo, db.DB.Create(repo).Error
}

var _ errutil.NotFound = (*ErrRepoNotExist)(nil)

type ErrRepoNotExist struct {
	args map[string]interface{}
}

func IsErrRepoNotExist(err error) bool {
	_, ok := err.(ErrRepoNotExist)
	return ok
}

func (err ErrRepoNotExist) Error() string {
	return fmt.Sprintf("repository does not exist: %v", err.args)
}

func (ErrRepoNotExist) NotFound() bool {
	return true
}

func (db *repos) GetByName(ownerID int64, name string) (*Repository, error) {
	repo := new(Repository)
	err := db.Where("owner_id = ? AND lower_name = ?", ownerID, strings.ToLower(name)).First(repo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRepoNotExist{args: map[string]interface{}{"ownerID": ownerID, "name": name}}
		}
		return nil, err
	}
	return repo, nil
}
