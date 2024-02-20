// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
)

func TestIssueReferencePattern(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    []string
	}{
		{
			name:    "no match",
			message: "Hello world!",
			want:    nil,
		},
		{
			name:    "contains issue numbers",
			message: "#123 is fixed, and #456 is WIP",
			want:    []string{"#123", " #456"},
		},
		{
			name:    "contains full issue references",
			message: "#123 is fixed, and user/repo#456 is WIP",
			want:    []string{"#123", " user/repo#456"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := issueReferencePattern.FindAllString(test.message, -1)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestAction_BeforeCreate(t *testing.T) {
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
		action := &Action{
			CreatedUnix: 1,
		}
		_ = action.BeforeCreate(db)
		assert.Equal(t, int64(1), action.CreatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		action := &Action{}
		_ = action.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), action.CreatedUnix)
	})
}

func TestAction_AfterFind(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	action := &Action{
		CreatedUnix: now.Unix(),
	}
	_ = action.AfterFind(db)
	assert.Equal(t, action.CreatedUnix, action.Created.Unix())
}

func TestActions(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ctx := context.Background()
	t.Parallel()
	db := &actionsStore{
		DB: newTestDB(t, "actionsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *actionsStore)
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
				err := clearTables(t, db.DB)
				require.NoError(t, err)
			})
			tc.test(t, ctx, db)
		})
		if t.Failed() {
			break
		}
	}
}

func actionsCommitRepo(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	now := time.Unix(1588568886, 0).UTC()

	conf.SetMockSSH(t, conf.SSHOpts{})

	t.Run("new commit", func(t *testing.T) {
		t.Cleanup(func() {
			err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(ctx).Delete(new(Action)).Error
			require.NoError(t, err)
		})

		err = db.CommitRepo(ctx,
			CommitRepoOptions{
				PusherName:  alice.Name,
				Owner:       alice,
				Repo:        repo,
				RefFullName: "refs/heads/main",
				OldCommitID: "ca82a6dff817ec66f44342007202690a93763949",
				NewCommitID: "085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7",
				Commits: CommitsToPushCommits(
					[]*git.Commit{
						{
							ID: git.MustIDFromString("085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7"),
							Author: &git.Signature{
								Name:  "alice",
								Email: "alice@example.com",
								When:  now,
							},
							Committer: &git.Signature{
								Name:  "alice",
								Email: "alice@example.com",
								When:  now,
							},
							Message: "A random commit",
						},
					},
				),
			},
		)
		require.NoError(t, err)

		got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
		require.NoError(t, err)
		require.Len(t, got, 1)
		got[0].ID = 0

		want := []*Action{
			{
				UserID:       alice.ID,
				OpType:       ActionCommitRepo,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				RefName:      "main",
				IsPrivate:    false,
				Content:      `{"Len":1,"Commits":[{"Sha1":"085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7","Message":"A random commit","AuthorEmail":"alice@example.com","AuthorName":"alice","CommitterEmail":"alice@example.com","CommitterName":"alice","Timestamp":"2020-05-04T05:08:06Z"}],"CompareURL":"alice/example/compare/ca82a6dff817ec66f44342007202690a93763949...085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7"}`,
				CreatedUnix:  db.NowFunc().Unix(),
			},
		}
		want[0].Created = time.Unix(want[0].CreatedUnix, 0)
		assert.Equal(t, want, got)
	})

	t.Run("new ref", func(t *testing.T) {
		t.Cleanup(func() {
			err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(ctx).Delete(new(Action)).Error
			require.NoError(t, err)
		})

		err = db.CommitRepo(ctx,
			CommitRepoOptions{
				PusherName:  alice.Name,
				Owner:       alice,
				Repo:        repo,
				RefFullName: "refs/heads/main",
				OldCommitID: git.EmptyID,
				NewCommitID: "085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7",
				Commits: CommitsToPushCommits(
					[]*git.Commit{
						{
							ID: git.MustIDFromString("085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7"),
							Author: &git.Signature{
								Name:  "alice",
								Email: "alice@example.com",
								When:  now,
							},
							Committer: &git.Signature{
								Name:  "alice",
								Email: "alice@example.com",
								When:  now,
							},
							Message: "A random commit",
						},
					},
				),
			},
		)
		require.NoError(t, err)

		got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
		require.NoError(t, err)
		require.Len(t, got, 2)
		got[0].ID = 0
		got[1].ID = 0

		want := []*Action{
			{
				UserID:       alice.ID,
				OpType:       ActionCommitRepo,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				RefName:      "main",
				IsPrivate:    false,
				Content:      `{"Len":1,"Commits":[{"Sha1":"085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7","Message":"A random commit","AuthorEmail":"alice@example.com","AuthorName":"alice","CommitterEmail":"alice@example.com","CommitterName":"alice","Timestamp":"2020-05-04T05:08:06Z"}],"CompareURL":""}`,
				CreatedUnix:  db.NowFunc().Unix(),
			},
			{
				UserID:       alice.ID,
				OpType:       ActionCreateBranch,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				RefName:      "main",
				IsPrivate:    false,
				Content:      `{"Len":1,"Commits":[{"Sha1":"085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7","Message":"A random commit","AuthorEmail":"alice@example.com","AuthorName":"alice","CommitterEmail":"alice@example.com","CommitterName":"alice","Timestamp":"2020-05-04T05:08:06Z"}],"CompareURL":""}`,
				CreatedUnix:  db.NowFunc().Unix(),
			},
		}
		want[0].Created = time.Unix(want[0].CreatedUnix, 0)
		want[1].Created = time.Unix(want[1].CreatedUnix, 0)
		assert.Equal(t, want, got)
	})

	t.Run("delete ref", func(t *testing.T) {
		t.Cleanup(func() {
			err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(ctx).Delete(new(Action)).Error
			require.NoError(t, err)
		})

		err = db.CommitRepo(ctx,
			CommitRepoOptions{
				PusherName:  alice.Name,
				Owner:       alice,
				Repo:        repo,
				RefFullName: "refs/heads/main",
				OldCommitID: "ca82a6dff817ec66f44342007202690a93763949",
				NewCommitID: git.EmptyID,
			},
		)
		require.NoError(t, err)

		got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
		require.NoError(t, err)
		require.Len(t, got, 1)
		got[0].ID = 0

		want := []*Action{
			{
				UserID:       alice.ID,
				OpType:       ActionDeleteBranch,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				RefName:      "main",
				IsPrivate:    false,
				CreatedUnix:  db.NowFunc().Unix(),
			},
		}
		want[0].Created = time.Unix(want[0].CreatedUnix, 0)
		assert.Equal(t, want, got)
	})
}

func actionsListByOrganization(t *testing.T, ctx context.Context, db *actionsStore) {
	if os.Getenv("GOGS_DATABASE_TYPE") != "postgres" {
		t.Skip("Skipping testing with not using PostgreSQL")
		return
	}

	conf.SetMockUI(t,
		conf.UIOpts{
			User: conf.UIUserOpts{
				NewsFeedPagingNum: 20,
			},
		},
	)

	tests := []struct {
		name    string
		orgID   int64
		actorID int64
		afterID int64
		want    string
	}{
		{
			name:    "no afterID",
			orgID:   1,
			actorID: 1,
			afterID: 0,
			want:    `SELECT * FROM "action" WHERE user_id = 1 AND (true OR id < 0) AND repo_id IN (SELECT repository.id FROM "repository" JOIN team_repo ON repository.id = team_repo.repo_id WHERE team_repo.team_id IN (SELECT team_id FROM "team_user" WHERE team_user.org_id = 1 AND uid = 1) OR (repository.is_private = false AND repository.is_unlisted = false)) ORDER BY id DESC LIMIT 20`,
		},
		{
			name:    "has afterID",
			orgID:   1,
			actorID: 1,
			afterID: 5,
			want:    `SELECT * FROM "action" WHERE user_id = 1 AND (false OR id < 5) AND repo_id IN (SELECT repository.id FROM "repository" JOIN team_repo ON repository.id = team_repo.repo_id WHERE team_repo.team_id IN (SELECT team_id FROM "team_user" WHERE team_user.org_id = 1 AND uid = 1) OR (repository.is_private = false AND repository.is_unlisted = false)) ORDER BY id DESC LIMIT 20`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := db.DB.ToSQL(func(tx *gorm.DB) *gorm.DB {
				return NewActionsStore(tx).(*actionsStore).listByOrganization(ctx, test.orgID, test.actorID, test.afterID).Find(new(Action))
			})
			assert.Equal(t, test.want, got)
		})
	}
}

func actionsListByUser(t *testing.T, ctx context.Context, db *actionsStore) {
	if os.Getenv("GOGS_DATABASE_TYPE") != "postgres" {
		t.Skip("Skipping testing with not using PostgreSQL")
		return
	}

	conf.SetMockUI(t,
		conf.UIOpts{
			User: conf.UIUserOpts{
				NewsFeedPagingNum: 20,
			},
		},
	)

	tests := []struct {
		name      string
		userID    int64
		actorID   int64
		afterID   int64
		isProfile bool
		want      string
	}{
		{
			name:      "same user no afterID not in profile",
			userID:    1,
			actorID:   1,
			afterID:   0,
			isProfile: false,
			want:      `SELECT * FROM "action" WHERE user_id = 1 AND (true OR id < 0) AND (true OR (is_private = false AND act_user_id = 1)) ORDER BY id DESC LIMIT 20`,
		},
		{
			name:      "same user no afterID in profile",
			userID:    1,
			actorID:   1,
			afterID:   0,
			isProfile: true,
			want:      `SELECT * FROM "action" WHERE user_id = 1 AND (true OR id < 0) AND (true OR (is_private = false AND act_user_id = 1)) ORDER BY id DESC LIMIT 20`,
		},
		{
			name:      "same user has afterID not in profile",
			userID:    1,
			actorID:   1,
			afterID:   5,
			isProfile: false,
			want:      `SELECT * FROM "action" WHERE user_id = 1 AND (false OR id < 5) AND (true OR (is_private = false AND act_user_id = 1)) ORDER BY id DESC LIMIT 20`,
		},
		{
			name:      "different user no afterID in profile",
			userID:    1,
			actorID:   2,
			afterID:   0,
			isProfile: true,
			want:      `SELECT * FROM "action" WHERE user_id = 1 AND (true OR id < 0) AND (false OR (is_private = false AND act_user_id = 1)) ORDER BY id DESC LIMIT 20`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := db.DB.ToSQL(func(tx *gorm.DB) *gorm.DB {
				return NewActionsStore(tx).(*actionsStore).listByUser(ctx, test.userID, test.actorID, test.afterID, test.isProfile).Find(new(Action))
			})
			assert.Equal(t, test.want, got)
		})
	}
}

func actionsMergePullRequest(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	err = db.MergePullRequest(ctx,
		alice,
		alice,
		repo,
		&Issue{
			Index: 1,
			Title: "Fix issue 1",
		},
	)
	require.NoError(t, err)

	got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	got[0].ID = 0

	want := []*Action{
		{
			UserID:       alice.ID,
			OpType:       ActionMergePullRequest,
			ActUserID:    alice.ID,
			ActUserName:  alice.Name,
			RepoID:       repo.ID,
			RepoUserName: alice.Name,
			RepoName:     repo.Name,
			IsPrivate:    false,
			Content:      `1|Fix issue 1`,
			CreatedUnix:  db.NowFunc().Unix(),
		},
	}
	want[0].Created = time.Unix(want[0].CreatedUnix, 0)
	assert.Equal(t, want, got)
}

func actionsMirrorSyncCreate(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	err = db.MirrorSyncCreate(ctx,
		alice,
		repo,
		"main",
	)
	require.NoError(t, err)

	got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	got[0].ID = 0

	want := []*Action{
		{
			UserID:       alice.ID,
			OpType:       ActionMirrorSyncCreate,
			ActUserID:    alice.ID,
			ActUserName:  alice.Name,
			RepoID:       repo.ID,
			RepoUserName: alice.Name,
			RepoName:     repo.Name,
			RefName:      "main",
			IsPrivate:    false,
			CreatedUnix:  db.NowFunc().Unix(),
		},
	}
	want[0].Created = time.Unix(want[0].CreatedUnix, 0)
	assert.Equal(t, want, got)
}

func actionsMirrorSyncDelete(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	err = db.MirrorSyncDelete(ctx,
		alice,
		repo,
		"main",
	)
	require.NoError(t, err)

	got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	got[0].ID = 0

	want := []*Action{
		{
			UserID:       alice.ID,
			OpType:       ActionMirrorSyncDelete,
			ActUserID:    alice.ID,
			ActUserName:  alice.Name,
			RepoID:       repo.ID,
			RepoUserName: alice.Name,
			RepoName:     repo.Name,
			RefName:      "main",
			IsPrivate:    false,
			CreatedUnix:  db.NowFunc().Unix(),
		},
	}
	want[0].Created = time.Unix(want[0].CreatedUnix, 0)
	assert.Equal(t, want, got)
}

func actionsMirrorSyncPush(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	now := time.Unix(1588568886, 0).UTC()
	err = db.MirrorSyncPush(ctx,
		MirrorSyncPushOptions{
			Owner:       alice,
			Repo:        repo,
			RefName:     "main",
			OldCommitID: "ca82a6dff817ec66f44342007202690a93763949",
			NewCommitID: "085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7",
			Commits: CommitsToPushCommits(
				[]*git.Commit{
					{
						ID: git.MustIDFromString("085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7"),
						Author: &git.Signature{
							Name:  "alice",
							Email: "alice@example.com",
							When:  now,
						},
						Committer: &git.Signature{
							Name:  "alice",
							Email: "alice@example.com",
							When:  now,
						},
						Message: "A random commit",
					},
				},
			),
		},
	)
	require.NoError(t, err)

	got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	got[0].ID = 0

	want := []*Action{
		{
			UserID:       alice.ID,
			OpType:       ActionMirrorSyncPush,
			ActUserID:    alice.ID,
			ActUserName:  alice.Name,
			RepoID:       repo.ID,
			RepoUserName: alice.Name,
			RepoName:     repo.Name,
			RefName:      "main",
			IsPrivate:    false,
			Content:      `{"Len":1,"Commits":[{"Sha1":"085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7","Message":"A random commit","AuthorEmail":"alice@example.com","AuthorName":"alice","CommitterEmail":"alice@example.com","CommitterName":"alice","Timestamp":"2020-05-04T05:08:06Z"}],"CompareURL":"alice/example/compare/ca82a6dff817ec66f44342007202690a93763949...085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7"}`,
			CreatedUnix:  db.NowFunc().Unix(),
		},
	}
	want[0].Created = time.Unix(want[0].CreatedUnix, 0)
	assert.Equal(t, want, got)
}

func actionsNewRepo(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	t.Run("new repo", func(t *testing.T) {
		t.Cleanup(func() {
			err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(ctx).Delete(new(Action)).Error
			require.NoError(t, err)
		})

		err = db.NewRepo(ctx, alice, alice, repo)
		require.NoError(t, err)

		got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
		require.NoError(t, err)
		require.Len(t, got, 1)
		got[0].ID = 0

		want := []*Action{
			{
				UserID:       alice.ID,
				OpType:       ActionCreateRepo,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				IsPrivate:    false,
				CreatedUnix:  db.NowFunc().Unix(),
			},
		}
		want[0].Created = time.Unix(want[0].CreatedUnix, 0)
		assert.Equal(t, want, got)
	})

	t.Run("fork repo", func(t *testing.T) {
		t.Cleanup(func() {
			err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(ctx).Delete(new(Action)).Error
			require.NoError(t, err)
		})

		repo.IsFork = true
		err = db.NewRepo(ctx, alice, alice, repo)
		require.NoError(t, err)

		got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
		require.NoError(t, err)
		require.Len(t, got, 1)
		got[0].ID = 0

		want := []*Action{
			{
				UserID:       alice.ID,
				OpType:       ActionForkRepo,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				IsPrivate:    false,
				CreatedUnix:  db.NowFunc().Unix(),
			},
		}
		want[0].Created = time.Unix(want[0].CreatedUnix, 0)
		assert.Equal(t, want, got)
	})
}

func actionsPushTag(t *testing.T, ctx context.Context, db *actionsStore) {
	// NOTE: We set a noop mock here to avoid data race with other tests that writes
	// to the mock server because this function holds a lock.
	conf.SetMockServer(t, conf.ServerOpts{})

	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	t.Run("new tag", func(t *testing.T) {
		t.Cleanup(func() {
			err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(ctx).Delete(new(Action)).Error
			require.NoError(t, err)
		})

		err = db.PushTag(ctx,
			PushTagOptions{
				Owner:       alice,
				Repo:        repo,
				PusherName:  alice.Name,
				RefFullName: "refs/tags/v1.0.0",
				NewCommitID: "085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7",
			},
		)
		require.NoError(t, err)

		got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
		require.NoError(t, err)
		require.Len(t, got, 1)
		got[0].ID = 0

		want := []*Action{
			{
				UserID:       alice.ID,
				OpType:       ActionPushTag,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				RefName:      "v1.0.0",
				IsPrivate:    false,
				CreatedUnix:  db.NowFunc().Unix(),
			},
		}
		want[0].Created = time.Unix(want[0].CreatedUnix, 0)
		assert.Equal(t, want, got)
	})

	t.Run("delete tag", func(t *testing.T) {
		t.Cleanup(func() {
			err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).WithContext(ctx).Delete(new(Action)).Error
			require.NoError(t, err)
		})

		err = db.PushTag(ctx,
			PushTagOptions{
				Owner:       alice,
				Repo:        repo,
				PusherName:  alice.Name,
				RefFullName: "refs/tags/v1.0.0",
				NewCommitID: git.EmptyID,
			},
		)
		require.NoError(t, err)

		got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
		require.NoError(t, err)
		require.Len(t, got, 1)
		got[0].ID = 0

		want := []*Action{
			{
				UserID:       alice.ID,
				OpType:       ActionDeleteTag,
				ActUserID:    alice.ID,
				ActUserName:  alice.Name,
				RepoID:       repo.ID,
				RepoUserName: alice.Name,
				RepoName:     repo.Name,
				RefName:      "v1.0.0",
				IsPrivate:    false,
				CreatedUnix:  db.NowFunc().Unix(),
			},
		}
		want[0].Created = time.Unix(want[0].CreatedUnix, 0)
		assert.Equal(t, want, got)
	})
}

func actionsRenameRepo(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	err = db.RenameRepo(ctx, alice, alice, "oldExample", repo)
	require.NoError(t, err)

	got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	got[0].ID = 0

	want := []*Action{
		{
			UserID:       alice.ID,
			OpType:       ActionRenameRepo,
			ActUserID:    alice.ID,
			ActUserName:  alice.Name,
			RepoID:       repo.ID,
			RepoUserName: alice.Name,
			RepoName:     repo.Name,
			IsPrivate:    false,
			Content:      "oldExample",
			CreatedUnix:  db.NowFunc().Unix(),
		},
	}
	want[0].Created = time.Unix(want[0].CreatedUnix, 0)
	assert.Equal(t, want, got)
}

func actionsTransferRepo(t *testing.T, ctx context.Context, db *actionsStore) {
	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := NewUsersStore(db.DB).Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "example",
		},
	)
	require.NoError(t, err)

	err = db.TransferRepo(ctx, alice, alice, bob, repo)
	require.NoError(t, err)

	got, err := db.ListByUser(ctx, alice.ID, alice.ID, 0, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	got[0].ID = 0

	want := []*Action{
		{
			UserID:       alice.ID,
			OpType:       ActionTransferRepo,
			ActUserID:    alice.ID,
			ActUserName:  alice.Name,
			RepoID:       repo.ID,
			RepoUserName: bob.Name,
			RepoName:     repo.Name,
			IsPrivate:    false,
			Content:      "alice/example",
			CreatedUnix:  db.NowFunc().Unix(),
		},
	}
	want[0].Created = time.Unix(want[0].CreatedUnix, 0)
	assert.Equal(t, want, got)
}
