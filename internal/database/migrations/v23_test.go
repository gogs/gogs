// Copyright 2025 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type repositoryPreV22 struct {
	PullsAllowRebase      bool              `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
}

func (*repositoryPreV22) TableName() string {
	return "repository"
}

type repositoryV22 struct {
	PullsAllowAlt         bool              `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
}

func (*repositoryV22) TableName() string {
	return "repository"
}

func TestRenameRepoPullsAllowRebaseToPullsAllowAlt(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "renamePullsAllowRebaseToPullsAllowAlt", new(repositoryPreV22))
	err := db.Create(
		&repositoryPreV22{
			PullsAllowRebase: false,
		},
	).Error
	require.NoError(t, err)
	assert.False(t, db.Migrator().HasColumn(&repositoryV22{}, "PullsAllowAlt"))
	assert.True(t, db.Migrator().HasColumn(&repositoryPreV22{}, "PullsAllowRebase"))

	err = renameRepoPullsAllowRebaseToPullsAllowAlt(db)
	require.NoError(t, err)
	assert.True(t, db.Migrator().HasColumn(&repositoryV22{}, "PullsAllowAlt"))
	assert.False(t, db.Migrator().HasColumn(&repositoryPreV22{}, "PullsAllowRebase"))

	// Re-run should be skipped
	err = renameRepoPullsAllowRebaseToPullsAllowAlt(db)
	require.Equal(t, errMigrationSkipped, err)
}
