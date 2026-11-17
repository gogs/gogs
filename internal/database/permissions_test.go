package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

func TestPerms(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &PermissionsStore{
		db: newTestDB(t, "PermissionsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *PermissionsStore)
	}{
		{"AccessMode", permsAccessMode},
		{"AccessModes", permsAccessModes},
		{"Authorize", permsAuthorize},
		{"SetRepoPerms", permsSetRepoPerms},
		{"recalculateAccesses", permsRecalculateAccesses},
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

func permsAccessMode(t *testing.T, ctx context.Context, s *PermissionsStore) {
	// Set up permissions.
	err := s.SetRepoPerms(ctx, 1,
		map[int64]AccessMode{
			2: AccessModeWrite,
			3: AccessModeAdmin,
		},
	)
	require.NoError(t, err)
	err = s.SetRepoPerms(ctx, 2,
		map[int64]AccessMode{
			1: AccessModeRead,
		},
	)
	require.NoError(t, err)

	publicRepoID := int64(1)
	publicRepoOpts := AccessModeOptions{
		OwnerID: 98,
	}

	privateRepoID := int64(2)
	privateRepoOpts := AccessModeOptions{
		OwnerID: 99,
		Private: true,
	}

	tests := []struct {
		name           string
		userID         int64
		repoID         int64
		opts           AccessModeOptions
		wantAccessMode AccessMode
	}{
		{
			name:           "nil repository",
			wantAccessMode: AccessModeNone,
		},

		{
			name:           "anonymous user has read access to public repository",
			repoID:         publicRepoID,
			opts:           publicRepoOpts,
			wantAccessMode: AccessModeRead,
		},
		{
			name:           "anonymous user has no access to private repository",
			repoID:         privateRepoID,
			opts:           privateRepoOpts,
			wantAccessMode: AccessModeNone,
		},

		{
			name:           "user is the owner",
			userID:         98,
			repoID:         publicRepoID,
			opts:           publicRepoOpts,
			wantAccessMode: AccessModeOwner,
		},
		{
			name:           "user 1 has read access to public repo",
			userID:         1,
			repoID:         publicRepoID,
			opts:           publicRepoOpts,
			wantAccessMode: AccessModeRead,
		},
		{
			name:           "user 2 has write access to public repo",
			userID:         2,
			repoID:         publicRepoID,
			opts:           publicRepoOpts,
			wantAccessMode: AccessModeWrite,
		},
		{
			name:           "user 3 has admin access to public repo",
			userID:         3,
			repoID:         publicRepoID,
			opts:           publicRepoOpts,
			wantAccessMode: AccessModeAdmin,
		},

		{
			name:           "user 1 has read access to private repo",
			userID:         1,
			repoID:         privateRepoID,
			opts:           privateRepoOpts,
			wantAccessMode: AccessModeRead,
		},
		{
			name:           "user 2 has no access to private repo",
			userID:         2,
			repoID:         privateRepoID,
			opts:           privateRepoOpts,
			wantAccessMode: AccessModeNone,
		},
		{
			name:           "user 3 has no access to private repo",
			userID:         3,
			repoID:         privateRepoID,
			opts:           privateRepoOpts,
			wantAccessMode: AccessModeNone,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode := s.AccessMode(ctx, logx.NewNoopLogger(), test.userID, test.repoID, test.opts)
			assert.Equal(t, test.wantAccessMode, mode)
		})
	}
}

func permsAccessModes(t *testing.T, ctx context.Context, s *PermissionsStore) {
	err := s.SetRepoPerms(ctx, 1, map[int64]AccessMode{2: AccessModeWrite})
	require.NoError(t, err)
	err = s.SetRepoPerms(ctx, 2, map[int64]AccessMode{1: AccessModeRead})
	require.NoError(t, err)

	repos := []*Repository{
		{ID: 1, OwnerID: 98},
		{ID: 2, OwnerID: 99, IsPrivate: true},
		{ID: 3, OwnerID: 98, IsPrivate: true},
	}
	tests := []struct {
		name   string
		userID int64
		want   map[int64]AccessMode
	}{
		{
			name:   "anonymous user",
			userID: 0,
			want:   map[int64]AccessMode{1: AccessModeRead, 2: AccessModeNone, 3: AccessModeNone},
		},
		{
			name:   "owner",
			userID: 98,
			want:   map[int64]AccessMode{1: AccessModeOwner, 2: AccessModeNone, 3: AccessModeOwner},
		},
		{
			name:   "user with access to the private repository",
			userID: 1,
			want:   map[int64]AccessMode{1: AccessModeRead, 2: AccessModeRead, 3: AccessModeNone},
		},
		{
			name:   "user with write access to the public repository",
			userID: 2,
			want:   map[int64]AccessMode{1: AccessModeWrite, 2: AccessModeNone, 3: AccessModeNone},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := s.AccessModes(ctx, test.userID, repos)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func permsAuthorize(t *testing.T, ctx context.Context, s *PermissionsStore) {
	// Set up permissions.
	err := s.SetRepoPerms(ctx, 1,
		map[int64]AccessMode{
			1: AccessModeRead,
			2: AccessModeWrite,
			3: AccessModeAdmin,
		},
	)
	require.NoError(t, err)

	repo := &Repository{
		ID:      1,
		OwnerID: 98,
	}

	tests := []struct {
		name           string
		userID         int64
		desired        AccessMode
		wantAuthorized bool
	}{
		{
			name:           "user 1 has read and wants read",
			userID:         1,
			desired:        AccessModeRead,
			wantAuthorized: true,
		},
		{
			name:           "user 1 has read and wants write",
			userID:         1,
			desired:        AccessModeWrite,
			wantAuthorized: false,
		},

		{
			name:           "user 2 has write and wants read",
			userID:         2,
			desired:        AccessModeRead,
			wantAuthorized: true,
		},
		{
			name:           "user 2 has write and wants write",
			userID:         2,
			desired:        AccessModeWrite,
			wantAuthorized: true,
		},
		{
			name:           "user 2 has write and wants admin",
			userID:         2,
			desired:        AccessModeAdmin,
			wantAuthorized: false,
		},

		{
			name:           "user 3 has admin and wants read",
			userID:         3,
			desired:        AccessModeRead,
			wantAuthorized: true,
		},
		{
			name:           "user 3 has admin and wants write",
			userID:         3,
			desired:        AccessModeWrite,
			wantAuthorized: true,
		},
		{
			name:           "user 3 has admin and wants admin",
			userID:         3,
			desired:        AccessModeAdmin,
			wantAuthorized: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorized := s.Authorize(ctx, logx.NewNoopLogger(), test.userID, repo.ID, test.desired,
				AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
			assert.Equal(t, test.wantAuthorized, authorized)
		})
	}
}

func permsSetRepoPerms(t *testing.T, ctx context.Context, s *PermissionsStore) {
	for _, update := range []struct {
		repoID    int64
		accessMap map[int64]AccessMode
	}{
		{
			repoID: 1,
			accessMap: map[int64]AccessMode{
				1: AccessModeWrite,
				2: AccessModeWrite,
				3: AccessModeAdmin,
				4: AccessModeWrite,
			},
		},
		{
			repoID: 2,
			accessMap: map[int64]AccessMode{
				1: AccessModeWrite,
				2: AccessModeRead,
				4: AccessModeWrite,
				5: AccessModeWrite,
			},
		},
		{
			repoID: 1,
			accessMap: map[int64]AccessMode{
				2: AccessModeWrite,
				3: AccessModeAdmin,
			},
		},
		{
			repoID: 2,
			accessMap: map[int64]AccessMode{
				1: AccessModeWrite,
				2: AccessModeRead,
				5: AccessModeWrite,
			},
		},
	} {
		err := s.SetRepoPerms(ctx, update.repoID, update.accessMap)
		if err != nil {
			t.Fatal(err)
		}
	}

	var accesses []*Access
	err := s.db.Order("user_id, repo_id").Find(&accesses).Error
	require.NoError(t, err)

	// Ignore ID fields.
	for _, a := range accesses {
		a.ID = 0
	}

	wantAccesses := []*Access{
		{UserID: 1, RepoID: 2, Mode: AccessModeWrite},
		{UserID: 2, RepoID: 1, Mode: AccessModeWrite},
		{UserID: 2, RepoID: 2, Mode: AccessModeRead},
		{UserID: 3, RepoID: 1, Mode: AccessModeAdmin},
		{UserID: 5, RepoID: 2, Mode: AccessModeWrite},
	}
	assert.Equal(t, wantAccesses, accesses)

	t.Run("writes only the rows that differ", func(t *testing.T) {
		require.NoError(t, s.SetRepoPerms(ctx, 3, map[int64]AccessMode{1: AccessModeRead, 2: AccessModeRead}))
		before := map[int64]int64{}
		var rows []*Access
		require.NoError(t, s.db.Where("repo_id = 3").Find(&rows).Error)
		for _, a := range rows {
			before[a.UserID] = a.ID
		}

		require.NoError(t, s.SetRepoPerms(ctx, 3, map[int64]AccessMode{1: AccessModeRead, 2: AccessModeAdmin, 3: AccessModeWrite}))
		rows = nil
		require.NoError(t, s.db.Where("repo_id = 3").Find(&rows).Error)
		for _, a := range rows {
			if id, ok := before[a.UserID]; ok {
				assert.Equal(t, id, a.ID, "the row of user %d is kept", a.UserID)
			}
		}
		assert.Equal(t, map[int64]AccessMode{1: AccessModeRead, 2: AccessModeAdmin, 3: AccessModeWrite}, accessModes(t, ctx, s, 3))
	})

	t.Run("concurrent", func(t *testing.T) {
		want := map[int64]AccessMode{1: AccessModeRead, 2: AccessModeWrite, 3: AccessModeAdmin}
		concurrently(t, 8, func(int) error { return s.SetRepoPerms(ctx, 4, want) })
		assert.Equal(t, want, accessModes(t, ctx, s, 4))
	})

	t.Run("more rows than a statement can bind", func(t *testing.T) {
		want := make(map[int64]AccessMode, 25000)
		for userID := range int64(25000) {
			want[userID+1] = AccessModeRead
		}
		require.NoError(t, s.SetRepoPerms(ctx, 5, want))
		assert.Equal(t, want, accessModes(t, ctx, s, 5))

		require.NoError(t, s.SetRepoPerms(ctx, 5, map[int64]AccessMode{7: AccessModeWrite}))
		assert.Equal(t, map[int64]AccessMode{7: AccessModeWrite}, accessModes(t, ctx, s, 5))
	})
}

func permsRecalculateAccesses(t *testing.T, ctx context.Context, s *PermissionsStore) {
	users := map[string]*User{}
	for _, name := range []string{"pr-alice", "pr-bob", "pr-dave", "pr-erin", "pr-frank"} {
		users[name] = newTestIssueUser(t, ctx, s.db, name)
	}
	addCollaborator := func(repo *Repository, name string, mode AccessMode) {
		t.Helper()
		err := s.db.Create(&Collaboration{RepoID: repo.ID, UserID: users[name].ID, Mode: mode}).Error
		require.NoError(t, err)
	}

	t.Run("user", func(t *testing.T) {
		repo := newTestIssueRepo(t, ctx, s.db, "pr-user")
		other := newTestIssueRepo(t, ctx, s.db, "pr-user-other")
		addCollaborator(repo, "pr-alice", AccessModeRead)
		addCollaborator(repo, "pr-bob", AccessModeAdmin)
		require.NoError(t, s.SetRepoPerms(ctx, repo.ID, map[int64]AccessMode{users["pr-dave"].ID: AccessModeWrite}))
		require.NoError(t, s.SetRepoPerms(ctx, other.ID, map[int64]AccessMode{users["pr-dave"].ID: AccessModeWrite}))

		err := s.recalculateAccesses(ctx, repo)
		require.NoError(t, err)
		assert.Equal(t, map[int64]AccessMode{
			users["pr-alice"].ID: AccessModeRead,
			users["pr-bob"].ID:   AccessModeAdmin,
		}, accessModes(t, ctx, s, repo.ID))
		assert.Equal(t, map[int64]AccessMode{users["pr-dave"].ID: AccessModeWrite}, accessModes(t, ctx, s, other.ID))

		// A repository without collaborators has no accesses.
		err = s.recalculateAccesses(ctx, other)
		require.NoError(t, err)
		assert.Empty(t, accessModes(t, ctx, s, other.ID))

		err = s.recalculateTeamAccesses(ctx, repo, 0)
		assert.Error(t, err, "the owner is not an organization")
	})

	t.Run("organization", func(t *testing.T) {
		org := &User{Name: "pr-org", LowerName: "pr-org", Type: UserTypeOrganization}
		require.NoError(t, s.db.Create(org).Error)
		repo, err := newReposStore(s.db).Create(ctx, org.ID, CreateRepoOptions{Name: "repo1"})
		require.NoError(t, err)
		otherRepo, err := newReposStore(s.db).Create(ctx, org.ID, CreateRepoOptions{Name: "repo2"})
		require.NoError(t, err)

		newTeam := func(name string, authorize AccessMode, repos []*Repository, members ...string) *Team {
			t.Helper()
			team := &Team{OrgID: org.ID, Name: name, LowerName: name, Authorize: authorize}
			require.NoError(t, s.db.Create(team).Error)
			for _, r := range repos {
				require.NoError(t, s.db.Create(&TeamRepo{OrgID: org.ID, TeamID: team.ID, RepoID: r.ID}).Error)
			}
			for _, name := range members {
				require.NoError(t, s.db.Create(&TeamUser{OrgID: org.ID, TeamID: team.ID, UID: users[name].ID}).Error)
			}
			return team
		}
		// The owner team gets owner access, whatever mode it stores, without
		// being given the repository.
		newTeam(ownerTeamName, AccessModeRead, nil, "pr-dave")
		dev := newTeam("dev", AccessModeWrite, []*Repository{repo}, "pr-alice", "pr-erin")
		newTeam("readers", AccessModeRead, []*Repository{repo}, "pr-alice")
		newTeam("other", AccessModeAdmin, []*Repository{otherRepo}, "pr-frank")
		addCollaborator(repo, "pr-bob", AccessModeRead)
		addCollaborator(repo, "pr-erin", AccessModeAdmin)

		err = s.recalculateAccesses(ctx, repo)
		require.NoError(t, err)
		assert.Equal(t, map[int64]AccessMode{
			users["pr-alice"].ID: AccessModeWrite,
			users["pr-bob"].ID:   AccessModeRead,
			users["pr-dave"].ID:  AccessModeOwner,
			users["pr-erin"].ID:  AccessModeAdmin,
		}, accessModes(t, ctx, s, repo.ID))

		// The ignored team does not give access.
		err = s.recalculateTeamAccesses(ctx, repo, dev.ID)
		require.NoError(t, err)
		assert.Equal(t, map[int64]AccessMode{
			users["pr-alice"].ID: AccessModeRead,
			users["pr-bob"].ID:   AccessModeRead,
			users["pr-dave"].ID:  AccessModeOwner,
			users["pr-erin"].ID:  AccessModeAdmin,
		}, accessModes(t, ctx, s, repo.ID))

		err = s.recalculateAccesses(ctx, otherRepo)
		require.NoError(t, err)
		assert.Equal(t, map[int64]AccessMode{
			users["pr-dave"].ID:  AccessModeOwner,
			users["pr-frank"].ID: AccessModeAdmin,
		}, accessModes(t, ctx, s, otherRepo.ID))
	})
}
