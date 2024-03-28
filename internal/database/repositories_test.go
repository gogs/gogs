// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

	ctx := context.Background()
	s := &RepositoriesStore{
		db: newTestDB(t, "repos"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *RepositoriesStore)
	}{
		{"Create", reposCreate},
		{"GetByCollaboratorID", reposGetByCollaboratorID},
		{"GetByCollaboratorIDWithAccessMode", reposGetByCollaboratorIDWithAccessMode},
		{"GetByID", reposGetByID},
		{"GetByName", reposGetByName},
		{"Star", reposStar},
		{"Touch", reposTouch},
		{"ListByRepo", reposListWatches},
		{"Watch", reposWatch},
		{"HasForkedBy", reposHasForkedBy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

func reposCreate(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	t.Run("name not allowed", func(t *testing.T) {
		_, err := s.Create(ctx,
			1,
			CreateRepoOptions{
				Name: "my.git",
			},
		)
		wantErr := ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "pattern": "*.git"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("already exists", func(t *testing.T) {
		_, err := s.Create(ctx, 2,
			CreateRepoOptions{
				Name: "repo1",
			},
		)
		require.NoError(t, err)

		_, err = s.Create(ctx, 2,
			CreateRepoOptions{
				Name: "repo1",
			},
		)
		wantErr := ErrRepoAlreadyExist{args: errutil.Args{"ownerID": int64(2), "name": "repo1"}}
		assert.Equal(t, wantErr, err)
	})

	repo, err := s.Create(ctx, 3,
		CreateRepoOptions{
			Name: "repo2",
		},
	)
	require.NoError(t, err)

	repo, err = s.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), repo.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, 1, repo.NumWatches) // The owner is watching the repo by default.
}

func reposGetByCollaboratorID(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.Create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.Create(ctx, 2, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)

	permissionsStore := newPermissionsStore(s.db)
	err = permissionsStore.SetRepoPerms(ctx, repo1.ID, map[int64]AccessMode{3: AccessModeRead})
	require.NoError(t, err)
	err = permissionsStore.SetRepoPerms(ctx, repo2.ID, map[int64]AccessMode{4: AccessModeAdmin})
	require.NoError(t, err)

	t.Run("user 3 is a collaborator of repo1", func(t *testing.T) {
		got, err := s.GetByCollaboratorID(ctx, 3, 10, "")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, repo1.ID, got[0].ID)
	})

	t.Run("do not return directly owned repository", func(t *testing.T) {
		got, err := s.GetByCollaboratorID(ctx, 1, 10, "")
		require.NoError(t, err)
		require.Len(t, got, 0)
	})
}

func reposGetByCollaboratorIDWithAccessMode(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.Create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.Create(ctx, 2, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)
	repo3, err := s.Create(ctx, 2, CreateRepoOptions{Name: "repo3"})
	require.NoError(t, err)

	permissionsStore := newPermissionsStore(s.db)
	err = permissionsStore.SetRepoPerms(ctx, repo1.ID, map[int64]AccessMode{3: AccessModeRead})
	require.NoError(t, err)
	err = permissionsStore.SetRepoPerms(ctx, repo2.ID, map[int64]AccessMode{3: AccessModeAdmin, 4: AccessModeWrite})
	require.NoError(t, err)
	err = permissionsStore.SetRepoPerms(ctx, repo3.ID, map[int64]AccessMode{4: AccessModeWrite})
	require.NoError(t, err)

	got, err := s.GetByCollaboratorIDWithAccessMode(ctx, 3)
	require.NoError(t, err)
	require.Len(t, got, 2)

	accessModes := make(map[int64]AccessMode)
	for repo, mode := range got {
		accessModes[repo.ID] = mode
	}
	assert.Equal(t, AccessModeRead, accessModes[repo1.ID])
	assert.Equal(t, AccessModeAdmin, accessModes[repo2.ID])
}

func reposGetByID(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.Create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)

	got, err := s.GetByID(ctx, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, repo1.Name, got.Name)

	_, err = s.GetByID(ctx, 404)
	wantErr := ErrRepoNotExist{args: errutil.Args{"repoID": int64(404)}}
	assert.Equal(t, wantErr, err)
}

func reposGetByName(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo, err := s.Create(ctx, 1,
		CreateRepoOptions{
			Name: "repo1",
		},
	)
	require.NoError(t, err)

	_, err = s.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)

	_, err = s.GetByName(ctx, 1, "bad_name")
	wantErr := ErrRepoNotExist{args: errutil.Args{"ownerID": int64(1), "name": "bad_name"}}
	assert.Equal(t, wantErr, err)
}

func reposStar(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.Create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = s.Star(ctx, alice.ID, repo1.ID)
	require.NoError(t, err)

	repo1, err = s.GetByID(ctx, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, repo1.NumStars)

	alice, err = usersStore.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, alice.NumStars)
}

func reposTouch(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo, err := s.Create(ctx, 1,
		CreateRepoOptions{
			Name: "repo1",
		},
	)
	require.NoError(t, err)

	err = s.db.WithContext(ctx).Model(new(Repository)).Where("id = ?", repo.ID).Update("is_bare", true).Error
	require.NoError(t, err)

	// Make sure it is bare
	got, err := s.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.True(t, got.IsBare)

	// Touch it
	err = s.Touch(ctx, repo.ID)
	require.NoError(t, err)

	// It should not be bare anymore
	got, err = s.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.False(t, got.IsBare)
}

func reposListWatches(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	err := s.Watch(ctx, 1, 1)
	require.NoError(t, err)
	err = s.Watch(ctx, 2, 1)
	require.NoError(t, err)
	err = s.Watch(ctx, 2, 2)
	require.NoError(t, err)

	got, err := s.ListWatches(ctx, 1)
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

func reposWatch(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	reposStore := newReposStore(s.db)
	repo1, err := reposStore.Create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)

	err = s.Watch(ctx, 2, repo1.ID)
	require.NoError(t, err)

	// It is OK to watch multiple times and just be noop.
	err = s.Watch(ctx, 2, repo1.ID)
	require.NoError(t, err)

	repo1, err = reposStore.GetByID(ctx, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, repo1.NumWatches) // The owner is watching the repo by default.
}

func reposHasForkedBy(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	has := s.HasForkedBy(ctx, 1, 2)
	assert.False(t, has)

	_, err := newReposStore(s.db).Create(
		ctx,
		2,
		CreateRepoOptions{
			Name:   "repo1",
			ForkID: 1,
		},
	)
	require.NoError(t, err)

	has = s.HasForkedBy(ctx, 1, 2)
	assert.True(t, has)
}
