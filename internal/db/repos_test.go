// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/errutil"
)

func TestRepos(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(Repository)}
	db := &repos{
		DB: initTestDB(t, "repos", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *repos)
	}{
		{"create", reposCreate},
		{"GetByName", reposGetByName},
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

func reposCreate(t *testing.T, db *repos) {
	ctx := context.Background()

	t.Run("name not allowed", func(t *testing.T) {
		_, err := db.create(ctx,
			1,
			createRepoOpts{
				Name: "my.git",
			},
		)
		wantErr := ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "pattern": "*.git"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("already exists", func(t *testing.T) {
		_, err := db.create(ctx, 2,
			createRepoOpts{
				Name: "repo1",
			},
		)
		require.NoError(t, err)

		_, err = db.create(ctx, 2,
			createRepoOpts{
				Name: "repo1",
			},
		)
		wantErr := ErrRepoAlreadyExist{args: errutil.Args{"ownerID": int64(2), "name": "repo1"}}
		assert.Equal(t, wantErr, err)
	})

	repo, err := db.create(ctx, 3,
		createRepoOpts{
			Name: "repo2",
		},
	)
	require.NoError(t, err)

	repo, err = db.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), repo.Created.UTC().Format(time.RFC3339))
}

func reposGetByName(t *testing.T, db *repos) {
	ctx := context.Background()

	repo, err := db.create(ctx, 1,
		createRepoOpts{
			Name: "repo1",
		},
	)
	require.NoError(t, err)

	_, err = db.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)

	_, err = db.GetByName(ctx, 1, "bad_name")
	wantErr := ErrRepoNotExist{args: errutil.Args{"ownerID": int64(1), "name": "bad_name"}}
	assert.Equal(t, wantErr, err)
}
