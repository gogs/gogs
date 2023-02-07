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

func TestWatches(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []any{new(Watch), new(Repository)}
	db := &watches{
		DB: dbtest.NewDB(t, "watches", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *watches)
	}{
		{"ListByRepo", watchesListByRepo},
		{"Watch", watchesWatch},
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

func watchesListByRepo(t *testing.T, db *watches) {
	ctx := context.Background()

	err := db.Watch(ctx, 1, 1)
	require.NoError(t, err)
	err = db.Watch(ctx, 2, 1)
	require.NoError(t, err)
	err = db.Watch(ctx, 2, 2)
	require.NoError(t, err)

	got, err := db.ListByRepo(ctx, 1)
	require.NoError(t, err)
	for _, w := range got {
		w.ID = 0
	}

	want := []*Watch{
		{UserID: 1, RepoID: 1},
		{UserID: 2, RepoID: 1},
	}
	assert.Equal(t, want, got)
}

func watchesWatch(t *testing.T, db *watches) {
	ctx := context.Background()

	reposStore := NewReposStore(db.DB)
	repo1, err := reposStore.Create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)

	err = db.Watch(ctx, 2, repo1.ID)
	require.NoError(t, err)

	// It is OK to watch multiple times and just be noop.
	err = db.Watch(ctx, 2, repo1.ID)
	require.NoError(t, err)

	repo1, err = reposStore.GetByID(ctx, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, repo1.NumWatches) // The owner is watching the repo by default.
}
