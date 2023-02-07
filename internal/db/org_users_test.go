// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

func TestOrgUsers(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []any{new(OrgUser)}
	db := &orgUsers{
		DB: dbtest.NewDB(t, "orgUsers", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *orgUsers)
	}{
		{"CountByUser", orgUsersCountByUser},
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

func orgUsersCountByUser(t *testing.T, db *orgUsers) {
	ctx := context.Background()

	// TODO: Use Orgs.Join to replace SQL hack when the method is available.
	err := db.Exec(`INSERT INTO org_user (uid, org_id) VALUES (?, ?)`, 1, 1).Error
	require.NoError(t, err)
	err = db.Exec(`INSERT INTO org_user (uid, org_id) VALUES (?, ?)`, 2, 1).Error
	require.NoError(t, err)

	got, err := db.CountByUser(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)

	got, err = db.CountByUser(ctx, 404)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got)
}
