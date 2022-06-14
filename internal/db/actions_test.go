// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbtest"
)

func TestAction_BeforeCreate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("CreatedUnix has been set", func(t *testing.T) {
		action := &Action{CreatedUnix: 1}
		_ = action.BeforeCreate(db)
		assert.Equal(t, int64(1), action.CreatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		action := &Action{}
		_ = action.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), action.CreatedUnix)
	})
}

func TestActions(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []interface{}{new(Action)}
	db := &actions{
		DB: dbtest.NewDB(t, "actions", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *actions)
	}{
		{"CommitRepo", actionsCommitRepo},
		{"ListByOrganization", actionsListByOrganization},
		{"ListByUser", actionsListByUser},
		{"MergePullRequest", actionsMergePullRequest},
		{"MirrorSyncCreate", actionsMirrorSyncCreate},
		{"MirrorSyncDelete", actionsMirrorSyncDelete},
		{"MirrorSyncPush", actionsMirrorSyncPush},
		{"NewRepo", actionsNewRepo},
		{"PushTag", actionsPushTag},
		{"RenameRepo", actionsRenameRepo},
		{"TransferRepo", actionsTransferRepo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				require.NoError(t, err)
			})
			tc.test(t, db)
		})
		if t.Failed() {
			break
		}
	}
}

func actionsCommitRepo(t *testing.T, db *actions) {
	// todo
}

func actionsListByOrganization(t *testing.T, db *actions) {
	// todo
}

func actionsListByUser(t *testing.T, db *actions) {
	// todo
}

func actionsMergePullRequest(t *testing.T, db *actions) {
	// todo
}

func actionsMirrorSyncCreate(t *testing.T, db *actions) {
	// todo
}

func actionsMirrorSyncDelete(t *testing.T, db *actions) {
	// todo
}

func actionsMirrorSyncPush(t *testing.T, db *actions) {
	// todo
}

func actionsNewRepo(t *testing.T, db *actions) {
	// todo
}

func actionsPushTag(t *testing.T, db *actions) {
	// todo
}

func actionsRenameRepo(t *testing.T, db *actions) {
	// todo
}

func actionsTransferRepo(t *testing.T, db *actions) {
	// todo
}
