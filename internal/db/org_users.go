// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"

	"gorm.io/gorm"
)

// OrgUsersStore is the persistent interface for organization-user relations.
//
// NOTE: All methods are sorted in alphabetical order.
type OrgUsersStore interface {
	// CountByUser returns the number of organizations the user is a member of.
	CountByUser(ctx context.Context, userID int64) (int64, error)
}

var OrgUsers OrgUsersStore

var _ OrgUsersStore = (*orgUsers)(nil)

type orgUsers struct {
	*gorm.DB
}

// NewOrgUsersStore returns a persistent interface for organization-user
// relations with given database connection.
func NewOrgUsersStore(db *gorm.DB) OrgUsersStore {
	return &orgUsers{DB: db}
}

func (db *orgUsers) CountByUser(ctx context.Context, userID int64) (int64, error) {
	var count int64
	return count, db.WithContext(ctx).Model(&OrgUser{}).Where("uid = ?", userID).Count(&count).Error
}
