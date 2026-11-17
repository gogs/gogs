package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
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
		NumIssues:       5,
		NumClosedIssues: 2,
		NumPulls:        3,
		NumClosedPulls:  1,
		CreatedUnix:     now.Unix(),
		UpdatedUnix:     now.Unix(),
	}
	_ = repo.AfterFind(db)
	assert.Empty(t, repo.DefaultBranch)
	assert.Equal(t, 3, repo.NumOpenIssues)
	assert.Equal(t, 2, repo.NumOpenPulls)
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
		{"LoadAttributes", reposLoadAttributes},
		{"Star", reposStar},
		{"Unstar", reposUnstar},
		{"ListStargazersAndWatchers", reposListStargazersAndWatchers},
		{"Touch", reposTouch},
		{"ListByRepo", reposListWatches},
		{"Watch", reposWatch},
		{"Unwatch", reposUnwatch},
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
		wantErr := ErrNameNotAllowed{args: errx.Args{"reason": "reserved", "pattern": "*.git"}}
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
		wantErr := ErrRepoAlreadyExist{args: errx.Args{"ownerID": int64(2), "name": "repo1"}}
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
	wantErr := ErrRepoNotExist{args: errx.Args{"repoID": int64(404)}}
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
	wantErr := ErrRepoNotExist{args: errx.Args{"ownerID": int64(1), "name": "bad_name"}}
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

func reposUnstar(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.Create(ctx, 9999, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.Create(ctx, 9999, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)
	for _, star := range [][2]int64{{alice.ID, repo1.ID}, {alice.ID, repo2.ID}, {bob.ID, repo1.ID}} {
		require.NoError(t, s.Star(ctx, star[0], star[1]))
	}

	counts := func() []int {
		t.Helper()
		r1, err := s.GetByID(ctx, repo1.ID)
		require.NoError(t, err)
		r2, err := s.GetByID(ctx, repo2.ID)
		require.NoError(t, err)
		a, err := usersStore.GetByID(ctx, alice.ID)
		require.NoError(t, err)
		b, err := usersStore.GetByID(ctx, bob.ID)
		require.NoError(t, err)
		return []int{r1.NumStars, r2.NumStars, a.NumStars, b.NumStars}
	}
	assert.Equal(t, []int{2, 1, 2, 1}, counts())

	err = s.Unstar(ctx, alice.ID, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 1, 1, 1}, counts())
	assert.False(t, s.IsStarring(ctx, alice.ID, repo1.ID))
	assert.True(t, s.IsStarring(ctx, alice.ID, repo2.ID))
	assert.True(t, s.IsStarring(ctx, bob.ID, repo1.ID))

	// Unstarring a repository that the user does not star is a no-op.
	err = s.Unstar(ctx, alice.ID, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 1, 1, 1}, counts())

	assert.False(t, s.IsStarring(ctx, 0, repo1.ID))
}

func reposListStargazersAndWatchers(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.Create(ctx, 9999, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.Create(ctx, 9999, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)
	carol, err := usersStore.Create(ctx, "carol", "carol@example.com", CreateUserOptions{})
	require.NoError(t, err)

	require.NoError(t, s.Star(ctx, carol.ID, repo1.ID))
	require.NoError(t, s.Star(ctx, alice.ID, repo1.ID))
	require.NoError(t, s.Star(ctx, bob.ID, repo2.ID))
	require.NoError(t, s.Watch(ctx, bob.ID, repo1.ID))
	require.NoError(t, s.Watch(ctx, carol.ID, repo2.ID))

	userIDs := func(users []*User, err error) []int64 {
		t.Helper()
		require.NoError(t, err)
		ids := make([]int64, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		return ids
	}
	got, err := s.ListStargazers(ctx, repo1.ID, 1)
	assert.Equal(t, []int64{carol.ID, alice.ID}, userIDs(got, err))
	require.NotEmpty(t, got)
	assert.Equal(t, "carol", got[0].Name)
	assert.Empty(t, userIDs(s.ListStargazers(ctx, repo1.ID, 2)))
	assert.Equal(t, []int64{bob.ID}, userIDs(s.ListWatchers(ctx, repo1.ID, 1)))
	assert.Equal(t, []int64{carol.ID}, userIDs(s.ListWatchers(ctx, repo2.ID, 0)))
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

	// Make sure it is empty.
	got, err := s.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.True(t, got.IsEmpty)

	// Touch it
	err = s.Touch(ctx, repo.ID)
	require.NoError(t, err)

	// It should not be empty anymore.
	got, err = s.GetByName(ctx, repo.OwnerID, repo.Name)
	require.NoError(t, err)
	assert.False(t, got.IsEmpty)
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

func reposUnwatch(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.Create(ctx, 9999, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.Create(ctx, 9999, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)
	for _, repoID := range []int64{repo1.ID, repo2.ID} {
		require.NoError(t, s.Watch(ctx, 2, repoID))
		require.NoError(t, s.Watch(ctx, 3, repoID))
	}

	numWatches := func() []int {
		t.Helper()
		r1, err := s.GetByID(ctx, repo1.ID)
		require.NoError(t, err)
		r2, err := s.GetByID(ctx, repo2.ID)
		require.NoError(t, err)
		return []int{r1.NumWatches, r2.NumWatches}
	}
	// The owner watches the repository too.
	assert.Equal(t, []int{3, 3}, numWatches())

	err = s.Unwatch(ctx, 2, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 3}, numWatches())
	assert.False(t, s.IsWatching(ctx, 2, repo1.ID))
	assert.True(t, s.IsWatching(ctx, 2, repo2.ID))
	assert.True(t, s.IsWatching(ctx, 3, repo1.ID))

	// Unwatching a repository that the user does not watch is a no-op.
	err = s.Unwatch(ctx, 2, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 3}, numWatches())

	assert.False(t, s.IsWatching(ctx, 0, repo1.ID))
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

func reposLoadAttributes(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)
	cindy, err := usersStore.Create(ctx, "cindy", "cindy@example.com", CreateUserOptions{})
	require.NoError(t, err)

	root, err := s.Create(ctx, alice.ID, CreateRepoOptions{Name: "root"})
	require.NoError(t, err)
	fork, err := s.Create(ctx, bob.ID, CreateRepoOptions{Name: "fork", Fork: true, ForkID: root.ID})
	require.NoError(t, err)
	forkOfFork, err := s.Create(ctx, cindy.ID, CreateRepoOptions{Name: "fork-of-fork", Fork: true, ForkID: fork.ID})
	require.NoError(t, err)
	other, err := s.Create(ctx, alice.ID, CreateRepoOptions{Name: "other"})
	require.NoError(t, err)

	t.Run("fork chain", func(t *testing.T) {
		got, err := s.GetByID(ctx, forkOfFork.ID)
		require.NoError(t, err)
		gotOther, err := s.GetByID(ctx, other.ID)
		require.NoError(t, err)

		err = s.LoadAttributes(ctx, got, gotOther)
		require.NoError(t, err)
		assert.Equal(t, "cindy", got.Owner.Name)
		assert.Equal(t, "alice", gotOther.Owner.Name)
		require.NotNil(t, got.BaseRepo)
		assert.Equal(t, fork.ID, got.BaseRepo.ID)
		assert.Equal(t, "bob", got.BaseRepo.Owner.Name)
		require.NotNil(t, got.BaseRepo.BaseRepo)
		assert.Equal(t, root.ID, got.BaseRepo.BaseRepo.ID)
		assert.Equal(t, "alice", got.BaseRepo.BaseRepo.Owner.Name)
		assert.Nil(t, got.BaseRepo.BaseRepo.BaseRepo)
		assert.Nil(t, gotOther.BaseRepo)
	})

	t.Run("base repository in the same list", func(t *testing.T) {
		gotFork, err := s.GetByID(ctx, fork.ID)
		require.NoError(t, err)
		gotRoot, err := s.GetByID(ctx, root.ID)
		require.NoError(t, err)

		err = s.LoadAttributes(ctx, gotFork, gotRoot)
		require.NoError(t, err)
		assert.Same(t, gotRoot, gotFork.BaseRepo)
	})

	t.Run("loaded attributes are kept", func(t *testing.T) {
		got, err := s.GetByID(ctx, fork.ID)
		require.NoError(t, err)
		owner := &User{ID: bob.ID, Name: "preset"}
		got.Owner = owner

		err = s.LoadAttributes(ctx, got)
		require.NoError(t, err)
		assert.Same(t, owner, got.Owner)
	})

	t.Run("base repository is gone", func(t *testing.T) {
		orphan, err := s.Create(ctx, bob.ID, CreateRepoOptions{Name: "orphan", Fork: true, ForkID: 404})
		require.NoError(t, err)

		err = s.LoadAttributes(ctx, orphan)
		require.NoError(t, err)
		assert.False(t, orphan.IsFork)
		assert.Zero(t, orphan.ForkID)
		assert.Nil(t, orphan.BaseRepo)
	})

	t.Run("owner is gone", func(t *testing.T) {
		gone, err := s.Create(ctx, 404, CreateRepoOptions{Name: "gone"})
		require.NoError(t, err)

		err = s.LoadAttributes(ctx, gone)
		assert.True(t, IsErrUserNotExist(err), "got %v", err)
	})
}
