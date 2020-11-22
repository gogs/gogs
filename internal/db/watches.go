// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"

	"gorm.io/gorm"
)

// WatchesStore is the persistent interface for watches.
//
// NOTE: All methods are sorted in alphabetical order.
type WatchesStore interface {
	// ListByRepo returns all watches of the given repository.
	ListByRepo(ctx context.Context, repoID int64) ([]*Watch, error)
}

var Watches WatchesStore

var _ WatchesStore = (*watches)(nil)

type watches struct {
	*gorm.DB
}

func (db *watches) ListByRepo(ctx context.Context, repoID int64) ([]*Watch, error) {
	var watches []*Watch
	return watches, db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&watches).Error
}
