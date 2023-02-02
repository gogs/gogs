// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

func TestWatches(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []any{new(Watch)}
	db := &watches{
		DB: dbtest.NewDB(t, "watches", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *watches)
	}{
		{"ListByRepo", watchesListByRepo},
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

func watchesListByRepo(_ *testing.T, _ *watches) {
	// TODO: Add tests once WatchRepo is migrated to GORM.
}
