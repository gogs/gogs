// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type actionPreV21 struct {
	ID           int64 `gorm:"primaryKey"`
	UserID       int64
	OpType       int
	ActUserID    int64
	ActUserName  string
	RepoID       int64 `gorm:"index"`
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool `gorm:"not null;default:FALSE"`
	Content      string
	CreatedUnix  int64
}

func (*actionPreV21) TableName() string {
	return "action"
}

type actionV21 struct {
	ID           int64 `gorm:"primaryKey"`
	UserID       int64 `gorm:"index"`
	OpType       int
	ActUserID    int64
	ActUserName  string
	RepoID       int64 `gorm:"index"`
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool `gorm:"not null;default:FALSE"`
	Content      string
	CreatedUnix  int64
}

func (*actionV21) TableName() string {
	return "action"
}

func TestAddIndexToActionUserID(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "addIndexToActionUserID", new(actionPreV21))
	err := db.Create(
		&actionPreV21{
			ID:           1,
			UserID:       1,
			OpType:       1,
			ActUserID:    1,
			ActUserName:  "alice",
			RepoID:       1,
			RepoUserName: "alice",
			RepoName:     "example",
			RefName:      "main",
			IsPrivate:    false,
			CreatedUnix:  db.NowFunc().Unix(),
		},
	).Error
	require.NoError(t, err)
	assert.False(t, db.Migrator().HasIndex(&actionV21{}, "UserID"))

	err = addIndexToActionUserID(db)
	require.NoError(t, err)
	assert.True(t, db.Migrator().HasIndex(&actionV21{}, "UserID"))

	// Re-run should be skipped
	err = addIndexToActionUserID(db)
	require.Equal(t, errMigrationSkipped, err)
}
