// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"
	"time"

	"github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbtest"
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
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("CreatedUnix has been set", func(t *testing.T) {
		action := &Action{CreatedUnix: 1}
		_ = action.BeforeCreate(db)
		assert.Equal(t, int64(1), action.CreatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		action := &Action{}
		_ = action.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), action.CreatedUnix)
	})
}

func TestActions(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []interface{}{new(Action), new(User), new(Repository), new(EmailAddress), new(Watch)}
	db := &actions{
		DB: dbtest.NewDB(t, "actions", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *actions)
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

func actionsCommitRepo(t *testing.T, db *actions) {
	ctx := context.Background()

	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOpts{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		createRepoOpts{
			Name: "example",
		},
	)
	require.NoError(t, err)

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
							When:  db.NowFunc(),
						},
						Committer: &git.Signature{
							Name:  "alice",
							Email: "alice@example.com",
							When:  db.NowFunc(),
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

	want := []*Action{
		{
			ID:           1,
			UserID:       alice.ID,
			OpType:       ActionCommitRepo,
			ActUserID:    alice.ID,
			ActUserName:  alice.Name,
			RepoID:       repo.ID,
			RepoUserName: alice.Name,
			RepoName:     repo.Name,
			RefName:      "main",
			IsPrivate:    false,
			Content:      `{"Len":1,"Commits":[],"CompareURL":"alice/example/compare/ca82a6dff817ec66f44342007202690a93763949...085bb3bcb608e1e8451d4b2432f8ecbe6306e7e7"}`,
			CreatedUnix:  db.NowFunc().Unix(),
		},
	}
	want[0].Created = time.Unix(want[0].CreatedUnix, 0)
	assert.Equal(t, want, got)
}

func actionsListByOrganization(t *testing.T, db *actions) {
	// todo
}

func actionsListByUser(t *testing.T, db *actions) {
	// todo
}

func actionsMergePullRequest(t *testing.T, db *actions) {
	ctx := context.Background()

	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOpts{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		createRepoOpts{
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

	want := []*Action{
		{
			ID:           1,
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

func actionsMirrorSyncCreate(t *testing.T, db *actions) {
	ctx := context.Background()

	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOpts{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		createRepoOpts{
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

	want := []*Action{
		{
			ID:           1,
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

func actionsMirrorSyncDelete(t *testing.T, db *actions) {
	ctx := context.Background()

	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOpts{})
	require.NoError(t, err)
	repo, err := NewReposStore(db.DB).Create(ctx,
		alice.ID,
		createRepoOpts{
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

	want := []*Action{
		{
			ID:           1,
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

func actionsMirrorSyncPush(t *testing.T, db *actions) {
	// todo
}

func actionsNewRepo(t *testing.T, db *actions) {
	// todo
}

func actionsPushTag(t *testing.T, db *actions) {
	// todo
}

func actionsRenameRepo(t *testing.T, db *actions) {
	// todo
}

func actionsTransferRepo(t *testing.T, db *actions) {
	// todo
}
