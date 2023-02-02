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
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/errutil"
)

func TestRepository_BeforeCreate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("CreatedUnix has been set", func(t *testing.T) {
		repo := &Repository{
			CreatedUnix: 1,
		}
		_ = repo.BeforeCreate(db)
		assert.Equal(t, int64(1), repo.CreatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		repo := &Repository{}
		_ = repo.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), repo.CreatedUnix)
	})
}

func TestRepository_BeforeUpdate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	repo := &Repository{}
	_ = repo.BeforeUpdate(db)
	assert.Equal(t, db.NowFunc().Unix(), repo.UpdatedUnix)
}

func TestRepository_AfterFind(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	repo := &Repository{
		CreatedUnix: now.Unix(),
		UpdatedUnix: now.Unix(),
	}
	_ = repo.AfterFind(db)
	assert.Equal(t, repo.CreatedUnix, repo.Created.Unix())
	assert.Equal(t, repo.UpdatedUnix, repo.Updated.Unix())
}

func TestRepos(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []any{new(Repository)}
	db := &repos{
		DB: dbtest.NewDB(t, "repos", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *repos)
	}{
		{"Create", reposCreate},
		{"GetByName", reposGetByName},
		{"Touch", reposTouch},
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
		_, err := db.Create(ctx,
			1,
			CreateRepoOptions{
				Name: "my.git",
			},
		)
		wantErr := ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "pattern": "*.git"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("already exists", func(t *testing.T) {
		_, err := db.Create(ctx, 2,
			CreateRepoOptions{
				Name: "repo1",
			},
		)
		require.NoError(t, err)

		_, err = db.Create(ctx, 2,
			CreateRepoOptions{
				Name: "repo1",
			},
		)
		wantErr := ErrRepoAlreadyExist{args: errutil.Args{"ownerID": int64(2), "name": "repo1"}}
		assert.Equal(t, wantErr, err)
	})

	repo, err := db.Create(ctx, 3,
		CreateRepoOptions{
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

	repo, err := db.Create(ctx, 1,
		CreateRepoOptions{
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

func reposTouch(t *testing.T, db *repos) {
	ctx := context.Background()

	repo, err := db.Create(ctx, 1,
		CreateRepoOptions{
			Name: "repo1",
		},
	)
	require.NoError(t, err)

	err = db.WithContext(ctx).Model(new(Repository)).Where("id = ?", repo.ID).Update("is_bare", true).Error
	require.NoError(t, err)

	// Make sure it is bare
	got, err := db.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.True(t, got.IsBare)

	// Touch it
	err = db.Touch(ctx, repo.ID)
	require.NoError(t, err)

	// It should not be bare anymore
	got, err = db.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.False(t, got.IsBare)
}
