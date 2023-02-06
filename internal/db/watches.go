// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// WatchesStore is the persistent interface for watches.
//
// NOTE: All methods are sorted in alphabetical order.
type WatchesStore interface {
	// ListByRepo returns all watches of the given repository.
	ListByRepo(ctx context.Context, repoID int64) ([]*Watch, error)
	// Watch marks the user to watch the repository.
	Watch(ctx context.Context, userID, repoID int64) error
}

var Watches WatchesStore

var _ WatchesStore = (*watches)(nil)

type watches struct {
	*gorm.DB
}

// NewWatchesStore returns a persistent interface for watches with given
// database connection.
func NewWatchesStore(db *gorm.DB) WatchesStore {
	return &watches{DB: db}
}

func (db *watches) ListByRepo(ctx context.Context, repoID int64) ([]*Watch, error) {
	var watches []*Watch
	return watches, db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&watches).Error
}

func (db *watches) updateWatchingCount(tx *gorm.DB, repoID int64) error {
	/*
		Equivalent SQL for PostgreSQL:

		UPDATE repository
		SET num_watches = (
			SELECT COUNT(*) FROM watch WHERE repo_id = @repoID
		)
		WHERE id = @repoID
	*/
	return tx.Model(&Repository{}).
		Where("id = ?", repoID).
		Update(
			"num_watches",
			tx.Model(&Watch{}).Select("COUNT(*)").Where("repo_id = ?", repoID),
		).
		Error
}

func (db *watches) Watch(ctx context.Context, userID, repoID int64) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		w := &Watch{
			UserID: userID,
			RepoID: repoID,
		}
		result := tx.FirstOrCreate(w, w)
		if result.Error != nil {
			return errors.Wrap(result.Error, "upsert")
		} else if result.RowsAffected <= 0 {
			return nil // Relation already exists
		}

		return db.updateWatchingCount(tx, repoID)
	})
}
