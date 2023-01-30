// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbutil"
)

// OrgsStore is the persistent interface for organizations.
//
// NOTE: All methods are sorted in alphabetical order.
type OrgsStore interface {
	// List returns a list of organizations filtered by options.
	List(ctx context.Context, opts ListOrgsOptions) ([]*Organization, error)
}

var Orgs OrgsStore

var _ OrgsStore = (*orgs)(nil)

type orgs struct {
	*gorm.DB
}

// NewOrgsStore returns a persistent interface for orgs with given database
// connection.
func NewOrgsStore(db *gorm.DB) OrgsStore {
	return &orgs{DB: db}
}

type ListOrgsOptions struct {
	// Filter by the membership with the given user ID.
	MemberID int64
	// Whether to include private memberships.
	IncludePrivateMembers bool
}

func (db *orgs) List(ctx context.Context, opts ListOrgsOptions) ([]*Organization, error) {
	if opts.MemberID <= 0 {
		return nil, errors.New("MemberID must be greater than 0")
	}

	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "org"
		JOIN org_user ON org_user.org_id = org.id
		WHERE
			org_user.uid = @memberID
		[AND org_user.is_public = @includePrivateMembers]
		ORDER BY org.id ASC
	*/
	tx := db.WithContext(ctx).
		Joins(dbutil.Quote("JOIN org_user ON org_user.org_id = %s.id", "user")).
		Where("org_user.uid = ?", opts.MemberID).
		Order(dbutil.Quote("%s.id ASC", "user"))
	if !opts.IncludePrivateMembers {
		tx = tx.Where("org_user.is_public = ?", true)
	}

	var orgs []*Organization
	return orgs, tx.Find(&orgs).Error
}

type Organization = User

func (o *Organization) TableName() string {
	return "user"
}
