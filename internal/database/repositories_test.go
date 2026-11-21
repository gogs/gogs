package database

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/osx"
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
		{"ConcurrentCounts", reposConcurrentCounts},
		{"HasForkedBy", reposHasForkedBy},
		{"GetForkBy", reposGetForkBy},
		{"GetByRef", reposGetByRef},
		{"Count", reposCount},
		{"ListByPage", reposListByPage},
		{"FilterIDsWithIssues", reposFilterIDsWithIssues},
		{"ListMirrors", reposListMirrors},
		{"ListByOwner", reposListByOwner},
		{"ListOwnedAndCollaborative", reposListOwnedAndCollaborative},
		{"Search", reposSearch},
		{"ListAssigneesAndWriters", reposListAssigneesAndWriters},
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
		_, err := s.create(ctx,
			1,
			CreateRepoOptions{
				Name: "my.git",
			},
		)
		wantErr := ErrNameNotAllowed{args: errx.Args{"reason": "reserved", "pattern": "*.git"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("already exists", func(t *testing.T) {
		_, err := s.create(ctx, 2,
			CreateRepoOptions{
				Name: "repo1",
			},
		)
		require.NoError(t, err)

		_, err = s.create(ctx, 2,
			CreateRepoOptions{
				Name: "repo1",
			},
		)
		wantErr := ErrRepoAlreadyExist{args: errx.Args{"ownerID": int64(2), "name": "repo1"}}
		assert.Equal(t, wantErr, err)
	})

	repo, err := s.create(ctx, 3,
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
	repo1, err := s.create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.create(ctx, 2, CreateRepoOptions{Name: "repo2"})
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
	repo1, err := s.create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.create(ctx, 2, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)
	repo3, err := s.create(ctx, 2, CreateRepoOptions{Name: "repo3"})
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
	repo1, err := s.create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)

	got, err := s.GetByID(ctx, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, repo1.Name, got.Name)

	_, err = s.GetByID(ctx, 404)
	wantErr := ErrRepoNotExist{args: errx.Args{"repoID": int64(404)}}
	assert.Equal(t, wantErr, err)
}

func reposGetByName(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo, err := s.create(ctx, 1,
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
	repo1, err := s.create(ctx, 1, CreateRepoOptions{Name: "repo1"})
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
	repo1, err := s.create(ctx, 9999, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.create(ctx, 9999, CreateRepoOptions{Name: "repo2"})
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
	repo1, err := s.create(ctx, 9999, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.create(ctx, 9999, CreateRepoOptions{Name: "repo2"})
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
	repo, err := s.create(ctx, 1,
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
	repo1, err := reposStore.create(ctx, 1, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)

	err = s.Watch(ctx, 2, repo1.ID)
	require.NoError(t, err)

	// It is OK to watch multiple times and just be no-op.
	err = s.Watch(ctx, 2, repo1.ID)
	require.NoError(t, err)

	repo1, err = reposStore.GetByID(ctx, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, repo1.NumWatches) // The owner is watching the repo by default.
}

func reposUnwatch(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	repo1, err := s.create(ctx, 9999, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := s.create(ctx, 9999, CreateRepoOptions{Name: "repo2"})
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

	_, err := newReposStore(s.db).create(
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

	root, err := s.create(ctx, alice.ID, CreateRepoOptions{Name: "root"})
	require.NoError(t, err)
	fork, err := s.create(ctx, bob.ID, CreateRepoOptions{Name: "fork", Fork: true, ForkID: root.ID})
	require.NoError(t, err)
	forkOfFork, err := s.create(ctx, cindy.ID, CreateRepoOptions{Name: "fork-of-fork", Fork: true, ForkID: fork.ID})
	require.NoError(t, err)
	other, err := s.create(ctx, alice.ID, CreateRepoOptions{Name: "other"})
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
		orphan, err := s.create(ctx, bob.ID, CreateRepoOptions{Name: "orphan", Fork: true, ForkID: 404})
		require.NoError(t, err)

		err = s.LoadAttributes(ctx, orphan)
		require.NoError(t, err)
		assert.False(t, orphan.IsFork)
		assert.Zero(t, orphan.ForkID)
		assert.Nil(t, orphan.BaseRepo)
	})

	t.Run("owner is gone", func(t *testing.T) {
		gone, err := s.create(ctx, 404, CreateRepoOptions{Name: "gone"})
		require.NoError(t, err)

		err = s.LoadAttributes(ctx, gone)
		assert.True(t, IsErrUserNotExist(err), "got %v", err)
	})
}

func reposConcurrentCounts(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	const n = 10
	repo, err := s.create(ctx, 1, CreateRepoOptions{Name: "repo"})
	require.NoError(t, err)
	repos := make([]*Repository, n)
	for i := range repos {
		repos[i], err = s.create(ctx, 1, CreateRepoOptions{Name: fmt.Sprintf("repo%d", i)})
		require.NoError(t, err)
	}
	usersStore := newUsersStore(s.db)
	users := make([]*User, n)
	for i := range users {
		users[i], err = usersStore.Create(ctx, fmt.Sprintf("user%d", i), fmt.Sprintf("user%d@example.com", i), CreateUserOptions{})
		require.NoError(t, err)
		require.NoError(t, s.Watch(ctx, users[i].ID, repo.ID))
		require.NoError(t, s.Star(ctx, users[i].ID, repo.ID))
	}

	concurrently := func(do func(i int) error) {
		t.Helper()
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() { errs[i] = do(i) })
		}
		wg.Wait()
		for _, err := range errs {
			require.NoError(t, err)
		}
	}
	concurrently(func(i int) error { return s.Unwatch(ctx, users[i].ID, repo.ID) })
	concurrently(func(i int) error { return s.Unstar(ctx, users[i].ID, repo.ID) })
	// The counts match the rows.
	assertCounts := func(r *Repository) {
		t.Helper()
		got, err := s.GetByID(ctx, r.ID)
		require.NoError(t, err)
		var watches, stars int64
		require.NoError(t, s.db.Model(new(Watch)).Where("repo_id = ?", r.ID).Count(&watches).Error)
		require.NoError(t, s.db.Model(new(Star)).Where("repo_id = ?", r.ID).Count(&stars).Error)
		assert.Equal(t, []int{int(watches), int(stars)}, []int{got.NumWatches, got.NumStars}, "watches and stars of %s", r.Name)
	}
	assertCounts(repo)

	// SQLite lets only one transaction write, and watching reads first.
	if conf.UseSQLite3 {
		return
	}
	concurrently(func(i int) error { return s.Watch(ctx, users[i].ID, repos[i].ID) })
	for _, r := range repos {
		assertCounts(r)
	}
}

// newTestRepoOf creates a repository record of given name for the owner, and
// sets given columns on it.
func newTestRepoOf(t *testing.T, ctx context.Context, s *RepositoriesStore, owner *User, name string, columns map[string]any) *Repository {
	t.Helper()

	repo, err := s.create(ctx, owner.ID, CreateRepoOptions{Name: name, Description: "about " + name})
	require.NoError(t, err)
	if len(columns) > 0 {
		require.NoError(t, s.db.Model(repo).UpdateColumns(columns).Error)
	}
	repo, err = s.GetByID(ctx, repo.ID)
	require.NoError(t, err)
	return repo
}

func reposGetForkBy(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "gf-alice")
	bob := newTestIssueUser(t, ctx, s.db, "gf-bob")
	base := newTestRepoOf(t, ctx, s, alice, "base", nil)
	other := newTestRepoOf(t, ctx, s, alice, "other", nil)
	fork1 := newTestRepoOf(t, ctx, s, bob, "fork1", map[string]any{"is_fork": true, "fork_id": base.ID})
	fork2 := newTestRepoOf(t, ctx, s, alice, "fork2", map[string]any{"is_fork": true, "fork_id": base.ID})
	_ = newTestRepoOf(t, ctx, s, bob, "fork3", map[string]any{"is_fork": true, "fork_id": other.ID})

	got, err := s.GetForkBy(ctx, base.ID, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, fork1.ID, got.ID)
	assert.Equal(t, bob.ID, got.Owner.ID)
	assert.Equal(t, base.ID, got.BaseRepo.ID)

	_, err = s.GetForkBy(ctx, other.ID, alice.ID)
	assert.True(t, IsErrRepoNotExist(err), "got %v", err)

	forks, err := s.ListForks(ctx, base)
	require.NoError(t, err)
	assert.Equal(t, []int64{fork1.ID, fork2.ID}, repoIDs(forks))
	for _, fork := range forks {
		assert.Same(t, base, fork.BaseRepo)
	}
}

func reposGetByRef(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "gr-alice")
	repo := newTestRepoOf(t, ctx, s, alice, "Repo1", nil)

	got, err := s.GetByRef(ctx, "GR-Alice/repo1")
	require.NoError(t, err)
	assert.Equal(t, repo.ID, got.ID)
	assert.Equal(t, alice.ID, got.Owner.ID)

	for _, ref := range []string{"repo1", "a/repo1", "/repo1"} {
		_, err = s.GetByRef(ctx, ref)
		assert.IsType(t, InvalidRepoReference{}, err, "ref %q", ref)
	}
	_, err = s.GetByRef(ctx, "gr-nobody/repo1")
	assert.True(t, IsErrUserNotExist(err), "got %v", err)
	_, err = s.GetByRef(ctx, "gr-alice/repo2")
	assert.True(t, IsErrRepoNotExist(err), "got %v", err)
}

func reposCount(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "rc-alice")
	bob := newTestIssueUser(t, ctx, s.db, "rc-bob")
	_ = newTestRepoOf(t, ctx, s, alice, "public", nil)
	_ = newTestRepoOf(t, ctx, s, alice, "private", map[string]any{"is_private": true})
	_ = newTestRepoOf(t, ctx, s, bob, "public", nil)

	for _, tc := range []struct {
		ownerID        int64
		includePrivate bool
		want           int64
	}{
		{0, true, 3},
		{0, false, 2},
		{alice.ID, true, 2},
		{alice.ID, false, 1},
		{bob.ID, true, 1},
	} {
		got, err := s.Count(ctx, tc.ownerID, tc.includePrivate)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "ownerID %d includePrivate %v", tc.ownerID, tc.includePrivate)
	}
}

func reposListByPage(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "lp-alice")
	repo1 := newTestRepoOf(t, ctx, s, alice, "repo1", nil)
	repo2 := newTestRepoOf(t, ctx, s, alice, "repo2", map[string]any{"is_private": true})
	repo3 := newTestRepoOf(t, ctx, s, alice, "repo3", nil)

	got, err := s.ListByPage(ctx, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{repo1.ID, repo2.ID}, repoIDs(got))
	got, err = s.ListByPage(ctx, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{repo3.ID}, repoIDs(got))
}

func reposFilterIDsWithIssues(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "fi-alice")
	internal1 := newTestRepoOf(t, ctx, s, alice, "internal1", nil)
	disabled := newTestRepoOf(t, ctx, s, alice, "disabled", map[string]any{"enable_issues": false})
	external := newTestRepoOf(t, ctx, s, alice, "external", map[string]any{"enable_external_tracker": true})
	internal2 := newTestRepoOf(t, ctx, s, alice, "internal2", nil)

	got, err := s.FilterIDsWithIssues(ctx, []int64{internal2.ID, disabled.ID, external.ID, internal1.ID, 9999})
	require.NoError(t, err)
	assert.Equal(t, []int64{internal1.ID, internal2.ID}, got)

	got, err = s.FilterIDsWithIssues(ctx, []int64{disabled.ID, external.ID})
	require.NoError(t, err)
	assert.Nil(t, got)
	got, err = s.FilterIDsWithIssues(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func reposListMirrors(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "lm-alice")
	bob := newTestIssueUser(t, ctx, s.db, "lm-bob")
	regular := newTestRepoOf(t, ctx, s, alice, "regular", nil)
	mirror1 := newTestRepoOf(t, ctx, s, alice, "mirror1", map[string]any{"is_mirror": true})
	bobMirror := newTestRepoOf(t, ctx, s, bob, "mirror", map[string]any{"is_mirror": true})
	mirror2 := newTestRepoOf(t, ctx, s, alice, "mirror2", map[string]any{"is_mirror": true})
	bobRegular := newTestRepoOf(t, ctx, s, bob, "regular", nil)

	got, err := s.ListMirrorsByOwner(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{mirror1.ID, mirror2.ID}, repoIDs(got))

	got, err = s.ListNonMirrors(ctx)
	require.NoError(t, err)
	assert.Equal(t, []int64{regular.ID, bobRegular.ID}, repoIDs(got))
	_ = bobMirror
}

func reposListByOwner(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "lo-alice")
	bob := newTestIssueUser(t, ctx, s.db, "lo-bob")
	old := newTestRepoOf(t, ctx, s, alice, "old", map[string]any{"updated_unix": 100})
	recent := newTestRepoOf(t, ctx, s, alice, "recent", map[string]any{"updated_unix": 300})
	private := newTestRepoOf(t, ctx, s, alice, "private", map[string]any{"updated_unix": 200, "is_private": true})
	unlisted := newTestRepoOf(t, ctx, s, alice, "unlisted", map[string]any{"updated_unix": 400, "is_unlisted": true})
	_ = newTestRepoOf(t, ctx, s, bob, "other", map[string]any{"updated_unix": 500})

	got, err := s.ListByOwner(ctx, UserRepoOptions{UserID: alice.ID, Private: true})
	require.NoError(t, err)
	assert.Equal(t, []int64{unlisted.ID, recent.ID, private.ID, old.ID}, repoIDs(got))

	got, err = s.ListByOwner(ctx, UserRepoOptions{UserID: alice.ID})
	require.NoError(t, err)
	assert.Equal(t, []int64{recent.ID, old.ID}, repoIDs(got))

	got, err = s.ListByOwner(ctx, UserRepoOptions{UserID: alice.ID, Private: true, Page: 2, PageSize: 3})
	require.NoError(t, err)
	assert.Equal(t, []int64{old.ID}, repoIDs(got))
}

func reposListOwnedAndCollaborative(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "oc-alice")
	bob := newTestIssueUser(t, ctx, s.db, "oc-bob")
	own := newTestRepoOf(t, ctx, s, alice, "own", nil)
	shared := newTestRepoOf(t, ctx, s, bob, "shared", nil)
	unshared := newTestRepoOf(t, ctx, s, bob, "unshared", nil)
	require.NoError(t, s.db.Create(&Collaboration{RepoID: shared.ID, UserID: alice.ID, Mode: AccessModeWrite}).Error)
	require.NoError(t, s.db.Create(&Collaboration{RepoID: unshared.ID, UserID: 9999, Mode: AccessModeWrite}).Error)

	got, err := s.ListOwnedAndCollaborative(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{shared.ID, own.ID}, repoIDs(got))
}

func reposSearch(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "rs-alice")
	bob := newTestIssueUser(t, ctx, s.db, "rs-bob")
	public := newTestRepoOf(t, ctx, s, alice, "Gopher", nil)
	private := newTestRepoOf(t, ctx, s, alice, "gopher-private", map[string]any{"is_private": true})
	unlisted := newTestRepoOf(t, ctx, s, alice, "gopher-unlisted", map[string]any{"is_unlisted": true})
	publicWiki := newTestRepoOf(t, ctx, s, alice, "gopher-wiki", map[string]any{"is_private": true, "allow_public_wiki": true})
	bobRepo := newTestRepoOf(t, ctx, s, bob, "rust", map[string]any{"description": "not a gopher"})
	bobPrivate := newTestRepoOf(t, ctx, s, bob, "gopher-bob", map[string]any{"is_private": true})
	require.NoError(t, s.db.Create(&Access{RepoID: bobPrivate.ID, UserID: alice.ID, Mode: AccessModeRead}).Error)

	search := func(opts SearchRepoOptions) ([]int64, int64) {
		t.Helper()
		opts.OrderBy = "id ASC"
		if opts.PageSize == 0 {
			opts.PageSize = 10
		}
		repos, count, err := s.Search(ctx, opts)
		require.NoError(t, err)
		return repoIDs(repos), count
	}

	ids, count := search(SearchRepoOptions{Keyword: "gopher"})
	assert.Equal(t, []int64{public.ID, publicWiki.ID, bobRepo.ID}, ids)
	assert.Equal(t, int64(3), count)

	ids, count = search(SearchRepoOptions{Keyword: "GOPHER", Private: true})
	assert.Equal(t, []int64{public.ID, private.ID, unlisted.ID, publicWiki.ID, bobRepo.ID, bobPrivate.ID}, ids)
	assert.Equal(t, int64(6), count)

	// A user also finds the repositories of their own and those they can access.
	ids, _ = search(SearchRepoOptions{Keyword: "gopher", UserID: alice.ID})
	assert.Equal(t, []int64{public.ID, private.ID, unlisted.ID, publicWiki.ID, bobRepo.ID, bobPrivate.ID}, ids)
	ids, _ = search(SearchRepoOptions{Keyword: "gopher", UserID: bob.ID})
	assert.Equal(t, []int64{public.ID, publicWiki.ID, bobRepo.ID, bobPrivate.ID}, ids)

	ids, count = search(SearchRepoOptions{Keyword: "gopher", OwnerID: bob.ID, Private: true})
	assert.Equal(t, []int64{bobRepo.ID, bobPrivate.ID}, ids)
	assert.Equal(t, int64(2), count)

	ids, count = search(SearchRepoOptions{Keyword: "gopher", Private: true, Page: 2, PageSize: 4})
	assert.Equal(t, []int64{bobRepo.ID, bobPrivate.ID}, ids)
	assert.Equal(t, int64(6), count)
}

func reposListAssigneesAndWriters(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "aw-alice")
	bob := newTestIssueUser(t, ctx, s.db, "aw-bob")
	carol := newTestIssueUser(t, ctx, s.db, "aw-carol")
	repo := newTestRepoOf(t, ctx, s, alice, "repo1", nil)
	other := newTestRepoOf(t, ctx, s, alice, "repo2", nil)
	perms := newPermissionsStore(s.db)
	require.NoError(t, perms.SetRepoPerms(ctx, repo.ID, map[int64]AccessMode{carol.ID: AccessModeWrite, bob.ID: AccessModeRead}))
	require.NoError(t, perms.SetRepoPerms(ctx, other.ID, map[int64]AccessMode{bob.ID: AccessModeAdmin}))

	got, err := s.ListAssignees(ctx, repo)
	require.NoError(t, err)
	assert.Equal(t, []int64{bob.ID, carol.ID, alice.ID}, userIDs(got))
	got, err = s.ListWriters(ctx, repo)
	require.NoError(t, err)
	assert.Equal(t, []int64{carol.ID, alice.ID}, userIDs(got))

	// The owner is left out when it is an organization.
	require.NoError(t, s.db.Model(new(User)).Where("id = ?", alice.ID).UpdateColumn("type", UserTypeOrganization).Error)
	repo, err = s.GetByID(ctx, repo.ID)
	require.NoError(t, err)
	got, err = s.ListWriters(ctx, repo)
	require.NoError(t, err)
	assert.Equal(t, []int64{carol.ID}, userIDs(got))
}

func TestReposLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// NOTE: The suite does not run in parallel because it mocks global
	// configuration that other suites read without locks.

	ctx := context.Background()
	s := &RepositoriesStore{
		db: newTestDB(t, "reposLifecycle"),
	}
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir(), DefaultBranch: "main", MaxCreationLimit: -1})
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: t.TempDir()})

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *RepositoriesStore)
	}{
		{"Create", reposLifecycleCreate},
		{"Fork", reposLifecycleFork},
		{"Transfer", reposLifecycleTransfer},
		{"Rename", reposLifecycleRename},
		{"Update", reposLifecycleUpdate},
		{"Delete", reposLifecycleDelete},
		{"ConcurrentForks", reposLifecycleConcurrentForks},
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

// newTestGitRepo creates a repository of given name for the owner with its
// Git repository.
func newTestGitRepo(t *testing.T, ctx context.Context, s *RepositoriesStore, owner *User, opts CreateRepoOptionsLegacy) *Repository {
	t.Helper()

	repo, err := s.Create(ctx, logx.NewNoopLogger(), owner, owner, opts)
	require.NoError(t, err)
	return repo
}

// countActions returns the number of actions of given type on the repository
// in the feed of the user.
func countActions(t *testing.T, db *gorm.DB, userID, repoID int64, opType ActionType) int64 {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(new(Action)).Where("user_id = ? AND repo_id = ? AND op_type = ?", userID, repoID, opType).Count(&count).Error)
	return count
}

func reposLifecycleCreate(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	logger := logx.NewNoopLogger()
	alice := newTestIssueUser(t, ctx, s.db, "lc-alice")

	repo, err := s.Create(ctx, logger, alice, alice, CreateRepoOptionsLegacy{Name: "Repo1", Description: "The repo", IsPrivate: true})
	require.NoError(t, err)
	got, err := s.GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, "Repo1", got.Name)
	assert.Equal(t, "repo1", got.LowerName)
	assert.Equal(t, "The repo", got.Description)
	assert.True(t, got.IsPrivate)
	assert.True(t, got.IsEmpty)
	assert.Equal(t, "main", got.DefaultBranch)
	assert.True(t, got.EnableWiki)
	assert.True(t, got.EnableIssues)
	assert.True(t, got.EnablePulls)
	returned := *repo
	returned.Owner = nil
	assert.Equal(t, *got, returned, "the returned repository matches what was saved")
	assert.True(t, osx.IsDir(RepoPath(alice.Name, "Repo1")))
	assert.True(t, s.IsWatching(ctx, alice.ID, repo.ID))
	assert.Equal(t, int64(1), countActions(t, s.db, alice.ID, repo.ID, ActionCreateRepo))
	u := reloadUser(t, ctx, s.db, alice.ID)
	assert.Equal(t, 1, u.NumRepos)
	assert.True(t, u.LastRepoVisibility)

	mirror, err := s.Create(ctx, logger, alice, alice, CreateRepoOptionsLegacy{Name: "Mirror1", IsMirror: true})
	require.NoError(t, err)
	got, err = s.GetByID(ctx, mirror.ID)
	require.NoError(t, err)
	assert.Equal(t, "mirror1", got.LowerName)

	// The size is saved from the objects of the Git repository.
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = RepoPath(alice.Name, "Repo1")
	// Random content does not compress, so the object takes some KiB.
	content := make([]byte, 64<<10)
	_, _ = rand.Read(content)
	cmd.Stdin = bytes.NewReader(content)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output: %s", out)
	require.NoError(t, s.UpdateSize(ctx, repo))
	got, err = s.GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Positive(t, got.Size)

	has, err := s.Exists(ctx, alice, "REPO1")
	require.NoError(t, err)
	assert.True(t, has)
	has, err = s.Exists(ctx, alice, "repo2")
	require.NoError(t, err)
	assert.False(t, has)
	_ = newTestRepoOf(t, ctx, s, alice, "nodisk", nil)
	has, err = s.Exists(ctx, alice, "nodisk")
	require.NoError(t, err)
	assert.False(t, has, "a repository without its Git repository does not exist")

	_, err = s.Create(ctx, logger, alice, alice, CreateRepoOptionsLegacy{Name: "repo1"})
	assert.ErrorContains(t, err, "repository directory already exists")
	_, err = s.Create(ctx, logger, alice, alice, CreateRepoOptionsLegacy{Name: "repo2.wiki"})
	assert.True(t, IsErrNameNotAllowed(err), "got %v", err)
	assert.False(t, osx.Exist(RepoPath(alice.Name, "repo2.wiki")))

	limited := newTestIssueUser(t, ctx, s.db, "lc-limited")
	limited.MaxRepoCreation = 0
	_, err = s.Create(ctx, logger, limited, limited, CreateRepoOptionsLegacy{Name: "repo1"})
	assert.True(t, IsErrReachLimitOfRepo(err), "got %v", err)

	// Members of the owners team get access to repositories of the organization.
	org, owner := newTestOrg(t, ctx, s.db, "lc-org")
	orgRepo, err := s.Create(ctx, logger, owner, org, CreateRepoOptionsLegacy{Name: "repo1", IsPrivate: true})
	require.NoError(t, err)
	teams := newTeamsStore(s.db)
	owners, err := teams.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	assert.True(t, teams.HasRepo(ctx, org.ID, owners.ID, orgRepo.ID))
	assert.Equal(t, AccessModeOwner, accessModes(t, ctx, newPermissionsStore(s.db), orgRepo.ID)[owner.ID])
	assert.Equal(t, 1, reloadUser(t, ctx, s.db, org.ID).NumRepos)
}

func reposLifecycleFork(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	logger := logx.NewNoopLogger()
	alice := newTestIssueUser(t, ctx, s.db, "lf-alice")
	bob := newTestIssueUser(t, ctx, s.db, "lf-bob")
	base := newTestGitRepo(t, ctx, s, alice, CreateRepoOptionsLegacy{Name: "base", IsPrivate: true})

	fork, err := s.Fork(ctx, logger, bob, bob, base, "MyFork", "Forked")
	require.NoError(t, err)
	assert.False(t, fork.EnableWiki || fork.EnableIssues || fork.EnablePulls, "the returned fork has the feature switches it was created with")
	got, err := s.GetForkBy(ctx, base.ID, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, fork.ID, got.ID)
	assert.Equal(t, "MyFork", got.Name)
	assert.Equal(t, "Forked", got.Description)
	assert.True(t, got.IsFork)
	assert.True(t, got.IsPrivate)
	assert.Equal(t, base.DefaultBranch, got.DefaultBranch)
	assert.False(t, got.EnableWiki)
	assert.False(t, got.EnableIssues)
	assert.False(t, got.EnablePulls)
	assert.True(t, osx.IsDir(RepoPath(bob.Name, "MyFork")))
	assert.True(t, s.IsWatching(ctx, bob.ID, fork.ID))
	assert.Equal(t, int64(1), countActions(t, s.db, bob.ID, fork.ID, ActionForkRepo))
	assert.Equal(t, 1, reloadUser(t, ctx, s.db, bob.ID).NumRepos)
	base, err = s.GetByID(ctx, base.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, base.NumForks)

	limited := newTestIssueUser(t, ctx, s.db, "lf-limited")
	limited.MaxRepoCreation = 0
	_, err = s.Fork(ctx, logger, limited, limited, base, "fork", "")
	assert.True(t, IsErrReachLimitOfRepo(err), "got %v", err)
	_, err = s.Fork(ctx, logger, bob, bob, base, "myfork", "")
	assert.True(t, IsErrRepoAlreadyExist(err), "got %v", err)
}

func reposLifecycleTransfer(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	org, alice := newTestOrg(t, ctx, s.db, "lt-org")
	carol := newTestIssueUser(t, ctx, s.db, "lt-carol")
	dave := newTestIssueUser(t, ctx, s.db, "lt-dave")
	require.NoError(t, newOrganizationsStore(s.db).AddMember(ctx, org.ID, dave.ID))
	repo := newTestGitRepo(t, ctx, s, alice, CreateRepoOptionsLegacy{Name: "Moving"})
	require.NoError(t, os.MkdirAll(WikiPath(alice.Name, repo.Name), os.ModePerm))
	collaborations := newCollaborationsStore(s.db)
	require.NoError(t, collaborations.Add(ctx, repo, carol.ID))
	require.NoError(t, collaborations.Add(ctx, repo, dave.ID))

	require.NoError(t, s.Transfer(ctx, alice, "LT-Org", repo))
	got, err := s.GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, org.ID, got.OwnerID)
	assert.False(t, osx.Exist(RepoPath(alice.Name, repo.Name)))
	assert.True(t, osx.IsDir(RepoPath(org.Name, repo.Name)))
	assert.True(t, osx.IsDir(WikiPath(org.Name, repo.Name)))
	collaborators, err := collaborations.List(ctx, repo.ID)
	require.NoError(t, err)
	require.Len(t, collaborators, 1)
	assert.Equal(t, carol.ID, collaborators[0].ID, "members of the new owner are no longer collaborators")
	teams := newTeamsStore(s.db)
	owners, err := teams.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	assert.True(t, teams.HasRepo(ctx, org.ID, owners.ID, repo.ID))
	assert.Equal(t, 0, reloadUser(t, ctx, s.db, alice.ID).NumRepos)
	assert.Equal(t, 1, reloadUser(t, ctx, s.db, org.ID).NumRepos)
	assert.True(t, s.IsWatching(ctx, org.ID, repo.ID))
	assert.Equal(t, int64(1), countActions(t, s.db, alice.ID, repo.ID, ActionTransferRepo))

	// A transfer back removes the repository from the teams.
	got.Owner = org
	require.NoError(t, s.Transfer(ctx, alice, alice.Name, got))
	assert.False(t, teams.HasRepo(ctx, org.ID, owners.ID, repo.ID))
	assert.Equal(t, 0, reloadTeam(t, ctx, teams, owners.ID).NumRepos)
	assert.False(t, s.IsWatching(ctx, org.ID, repo.ID))
	assert.Equal(t, 0, reloadUser(t, ctx, s.db, org.ID).NumRepos)
	assert.True(t, osx.IsDir(RepoPath(alice.Name, repo.Name)))

	_ = newTestGitRepo(t, ctx, s, org, CreateRepoOptionsLegacy{Name: "moving"})
	err = s.Transfer(ctx, alice, org.Name, got)
	assert.True(t, IsErrRepoAlreadyExist(err), "got %v", err)
}

func reposLifecycleRename(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "lr-alice")
	_ = newTestGitRepo(t, ctx, s, alice, CreateRepoOptionsLegacy{Name: "before"})
	require.NoError(t, os.MkdirAll(WikiPath(alice.Name, "before"), os.ModePerm))
	_ = newTestGitRepo(t, ctx, s, alice, CreateRepoOptionsLegacy{Name: "taken"})

	require.NoError(t, s.Rename(ctx, alice, "Before", "After"))
	assert.False(t, osx.Exist(RepoPath(alice.Name, "before")))
	assert.True(t, osx.IsDir(RepoPath(alice.Name, "after")))
	assert.True(t, osx.IsDir(WikiPath(alice.Name, "after")))

	err := s.Rename(ctx, alice, "after", "TAKEN")
	assert.True(t, IsErrRepoAlreadyExist(err), "got %v", err)
	err = s.Rename(ctx, alice, "after", "after.git")
	assert.True(t, IsErrNameNotAllowed(err), "got %v", err)
	assert.True(t, osx.IsDir(RepoPath(alice.Name, "after")))
}

func reposLifecycleUpdate(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	logger := logx.NewNoopLogger()
	alice := newTestIssueUser(t, ctx, s.db, "lu-alice")
	bob := newTestIssueUser(t, ctx, s.db, "lu-bob")
	repo := newTestGitRepo(t, ctx, s, alice, CreateRepoOptionsLegacy{Name: "repo1"})
	fork, err := s.Fork(ctx, logger, bob, bob, repo, "fork", "")
	require.NoError(t, err)
	daemonExportFile := filepath.Join(repo.RepoPath(), "git-daemon-export-ok")

	// The lower name is derived from the name on every save.
	repo.Name = "REPO1"
	repo.LowerName = "stale"
	repo.Description = strings.Repeat("d", 600)
	repo.Website = strings.Repeat("w", 300)
	repo.IsPrivate = true
	require.NoError(t, s.Update(ctx, logger, repo, false))
	got, err := s.GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, "repo1", got.LowerName)
	assert.Equal(t, strings.Repeat("d", 512), got.Description)
	assert.Equal(t, strings.Repeat("w", 255), got.Website)
	assert.True(t, got.IsPrivate)
	got, err = s.GetByID(ctx, fork.ID)
	require.NoError(t, err)
	assert.True(t, got.IsPrivate, "forks follow a change of visibility that the caller does not report")

	require.NoError(t, os.WriteFile(daemonExportFile, nil, 0o644))
	require.NoError(t, s.Update(ctx, logger, repo, true))
	got, err = s.GetByID(ctx, fork.ID)
	require.NoError(t, err)
	assert.True(t, got.IsPrivate)
	var publicActions int64
	require.NoError(t, s.db.Model(new(Action)).Where("repo_id IN ? AND is_private = ?", []int64{repo.ID, fork.ID}, false).Count(&publicActions).Error)
	assert.Zero(t, publicActions)
	assert.False(t, osx.Exist(daemonExportFile))

	repo.IsPrivate = false
	repo.IsUnlisted = true
	require.NoError(t, s.Update(ctx, logger, repo, true))
	got, err = s.GetByID(ctx, fork.ID)
	require.NoError(t, err)
	assert.False(t, got.IsPrivate)
	assert.True(t, got.IsUnlisted)
	require.NoError(t, s.db.Model(new(Action)).Where("repo_id IN ? AND is_private = ?", []int64{repo.ID, fork.ID}, false).Count(&publicActions).Error)
	assert.Zero(t, publicActions, "actions of unlisted repositories are private")
	assert.True(t, osx.IsFile(daemonExportFile))
}

func reposLifecycleDelete(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	logger := logx.NewNoopLogger()
	setMockAttachmentPath(t)
	org, alice := newTestOrg(t, ctx, s.db, "ld-org")
	bob := newTestIssueUser(t, ctx, s.db, "ld-bob")
	repo := newTestGitRepo(t, ctx, s, org, CreateRepoOptionsLegacy{Name: "doomed"})
	kept := newTestGitRepo(t, ctx, s, org, CreateRepoOptionsLegacy{Name: "kept"})
	fork, err := s.Fork(ctx, logger, bob, bob, repo, "doomed-fork", "")
	require.NoError(t, err)
	require.NoError(t, s.Star(ctx, bob.ID, repo.ID))
	require.NoError(t, newCollaborationsStore(s.db).Add(ctx, repo, bob.ID))
	attachments := newAttachmentsStore(s.db)
	for _, r := range []*Repository{repo, kept} {
		issue := &Issue{RepoID: r.ID, Index: 1}
		require.NoError(t, s.db.Create(issue).Error)
		require.NoError(t, s.db.Create(&Comment{IssueID: issue.ID}).Error)
		_ = newTestAttachment(t, ctx, attachments, "file.txt", issue.ID, 0, 0)
	}
	var doomedIssueIDs []int64
	require.NoError(t, s.db.Model(new(Issue)).Where("repo_id = ?", repo.ID).Pluck("id", &doomedIssueIDs).Error)
	var doomedAttachment Attachment
	require.NoError(t, s.db.Joins("JOIN issue ON issue.id = attachment.issue_id").Where("issue.repo_id = ?", repo.ID).First(&doomedAttachment).Error)
	require.True(t, osx.IsFile(doomedAttachment.LocalPath()))

	err = s.Delete(ctx, logger, alice.ID, repo.ID)
	assert.True(t, IsErrRepoNotExist(err), "got %v", err)

	require.NoError(t, s.Delete(ctx, logger, org.ID, repo.ID))
	_, err = s.GetByID(ctx, repo.ID)
	assert.True(t, IsErrRepoNotExist(err), "got %v", err)
	assert.False(t, osx.Exist(RepoPath(org.Name, repo.Name)))
	assert.False(t, osx.Exist(doomedAttachment.LocalPath()))
	countOf := func(table any, query string, repoID int64) int64 {
		var count int64
		require.NoError(t, s.db.Model(table).Where(query, repoID).Count(&count).Error)
		return count
	}
	for _, tc := range []struct {
		table any
		query string
	}{
		{new(Issue), "repo_id = ?"},
		{new(Access), "repo_id = ?"},
		{new(Action), "repo_id = ?"},
		{new(Watch), "repo_id = ?"},
		{new(Star), "repo_id = ?"},
		{new(Collaboration), "repo_id = ?"},
		{new(TeamRepo), "repo_id = ?"},
	} {
		assert.Zero(t, countOf(tc.table, tc.query, repo.ID), "%T", tc.table)
		if _, ok := tc.table.(*Collaboration); !ok {
			if _, ok := tc.table.(*Star); !ok {
				assert.NotZero(t, countOf(tc.table, tc.query, kept.ID), "%T of another repository", tc.table)
			}
		}
	}
	for _, table := range []any{new(Comment), new(Attachment)} {
		var count int64
		require.NoError(t, s.db.Model(table).Where("issue_id IN ?", doomedIssueIDs).Count(&count).Error)
		assert.Zero(t, count, "%T", table)
	}
	assert.Equal(t, 1, reloadUser(t, ctx, s.db, org.ID).NumRepos)
	teams := newTeamsStore(s.db)
	owners, err := teams.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, owners.NumRepos)
	got, err := s.GetByID(ctx, fork.ID)
	require.NoError(t, err)
	assert.False(t, got.IsFork, "forks of a deleted repository stop being forks")
	assert.Zero(t, got.ForkID)

	// Deleting a fork decreases the number of forks of its base.
	fork2, err := s.Fork(ctx, logger, bob, bob, kept, "kept-fork", "")
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, logger, bob.ID, fork2.ID))
	kept, err = s.GetByID(ctx, kept.ID)
	require.NoError(t, err)
	assert.Zero(t, kept.NumForks)
}

func reposLifecycleConcurrentForks(t *testing.T, ctx context.Context, s *RepositoriesStore) {
	// SQLite lets only one transaction write, so the two cannot interleave.
	if conf.UseSQLite3 {
		t.Skip()
	}
	logger := logx.NewNoopLogger()
	alice := newTestIssueUser(t, ctx, s.db, "cf-alice")
	bob := newTestIssueUser(t, ctx, s.db, "cf-bob")
	carol := newTestIssueUser(t, ctx, s.db, "cf-carol")
	org, _ := newTestOrg(t, ctx, s.db, "cf-org")
	teams := newTeamsStore(s.db)
	readers := createTestTeam(t, ctx, teams, org.ID, "readers", AccessModeRead)
	member := newTestIssueUser(t, ctx, s.db, "cf-member")
	require.NoError(t, teams.AddMember(ctx, org.ID, readers.ID, member.ID))

	var n int
	name := func(prefix string) string {
		n++
		return fmt.Sprintf("%s%d", prefix, n)
	}
	fork := func(owner *User, base *Repository) (*Repository, error) {
		return s.Fork(ctx, logger, owner, owner, base, name("fork"), "")
	}
	update := func(repo *Repository) error {
		repo.IsPrivate = true
		return s.Update(ctx, logger, repo, true)
	}
	deleteRepo := func(repo *Repository) error { return s.Delete(ctx, logger, repo.OwnerID, repo.ID) }
	transfer := func(repo *Repository, to *User) error { return s.Transfer(ctx, repo.Owner, to.Name, repo) }

	for _, tc := range []struct {
		name      string
		baseOwner *User
		forkOwner *User // Defaults to bob.
		private   bool
		setup     func(base, fork *Repository)
		// first starts ahead of second, while the base repository is held.
		first, second func(base, fork *Repository) error
	}{
		{
			name:      "delete base while updating it",
			baseOwner: alice,
			first:     func(base, _ *Repository) error { return update(base) },
			second:    func(base, _ *Repository) error { return deleteRepo(base) },
		},
		{
			name:      "delete fork while updating base",
			baseOwner: alice,
			first:     func(base, _ *Repository) error { return update(base) },
			second:    func(_, fork *Repository) error { return deleteRepo(fork) },
		},
		{
			name:      "fork base while deleting another fork",
			baseOwner: alice,
			first:     func(_, fork *Repository) error { return deleteRepo(fork) },
			second:    func(base, _ *Repository) error { _, err := fork(carol, base); return err },
		},
		{
			name:      "fork base into an organization while deleting its other fork",
			baseOwner: alice,
			forkOwner: org,
			first:     func(base, _ *Repository) error { _, err := fork(org, base); return err },
			second:    func(_, fork *Repository) error { return deleteRepo(fork) },
		},
		{
			name:      "transfer fork into the organization while deleting its base",
			baseOwner: org,
			first:     func(base, _ *Repository) error { return deleteRepo(base) },
			second:    func(_, fork *Repository) error { return transfer(fork, org) },
		},
		{
			name:      "fork a fork into the organization while deleting its base",
			baseOwner: org,
			first:     func(base, _ *Repository) error { return deleteRepo(base) },
			second:    func(_, f *Repository) error { _, err := fork(org, f); return err },
		},
		{
			name:      "remove repository from a team while deleting it",
			baseOwner: org,
			private:   true,
			setup:     func(base, _ *Repository) { require.NoError(t, teams.AddRepo(ctx, readers, base)) },
			first:     func(base, _ *Repository) error { return deleteRepo(base) },
			second:    func(base, _ *Repository) error { return teams.RemoveRepo(ctx, readers, base.ID) },
		},
		{
			name:      "add repository to a team while deleting its fork in the team",
			baseOwner: org,
			forkOwner: org,
			private:   true,
			setup:     func(_, fork *Repository) { require.NoError(t, teams.AddRepo(ctx, readers, fork)) },
			first:     func(_, fork *Repository) error { return deleteRepo(fork) },
			second:    func(base, _ *Repository) error { return teams.AddRepo(ctx, readers, base) },
		},
		{
			name:      "add repository to a team while deleting a fork of a user",
			baseOwner: org,
			private:   true,
			first:     func(_, fork *Repository) error { return deleteRepo(fork) },
			second:    func(base, _ *Repository) error { return teams.AddRepo(ctx, readers, base) },
		},
	} {
		base := newTestGitRepo(t, ctx, s, tc.baseOwner, CreateRepoOptionsLegacy{Name: name("base"), IsPrivate: tc.private})
		forkOwner := tc.forkOwner
		if forkOwner == nil {
			forkOwner = bob
		}
		baseFork, err := fork(forkOwner, base)
		require.NoError(t, err)
		if tc.setup != nil {
			tc.setup(base, baseFork)
		}

		hold := s.db.Begin()
		require.NoError(t, hold.Error)
		defer hold.Rollback()
		require.NoError(t, hold.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", base.ID).Find(new(Repository)).Error)

		var wg sync.WaitGroup
		var firstErr, secondErr error
		wg.Go(func() { firstErr = tc.first(base, baseFork) })
		time.Sleep(500 * time.Millisecond)
		wg.Go(func() { secondErr = tc.second(base, baseFork) })
		time.Sleep(500 * time.Millisecond)
		require.NoError(t, hold.Rollback().Error)
		wg.Wait()

		assert.NoError(t, firstErr, tc.name)
		assert.NoError(t, secondErr, tc.name)
	}
}
