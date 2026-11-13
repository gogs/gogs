package database

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/lfsx"
	"gogs.io/gogs/internal/testx"
)

func TestDumpAndImport(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	const wantTables = 36
	if len(Tables) != wantTables {
		t.Fatalf("New table has added (want %d got %d), please add new tests for the table and update this check", wantTables, len(Tables))
	}

	db := newTestDB(t, "dumpAndImport")
	setupDBToDump(t, db)
	dumpTables(t, db)
	importTables(t, db)

	// Dump and assert golden again to make sure data aren't changed.
	dumpTables(t, db)
}

func TestImportGogs012Rows(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := newTestDB(t, "importGogs012Rows")
	ctx := context.Background()

	// Rows as Gogs 0.12 writes them to backup archives.
	const sha1 = "bea0db4951d32f2d3c99e4394b4e6314882fa746"
	// The JSON encoder escapes HTML characters, so this content of 40 KB, which
	// fits into a TEXT column of MySQL, becomes a line of 140 KB.
	longContent := strings.Repeat("<br>", 10_000)
	longContentJSON := strings.Repeat(`\u003cbr\u003e`, 10_000)
	rows := []struct {
		table any
		line  string
	}{
		{new(User), `{"ID":1,"LowerName":"alice","Name":"alice","Email":"alice@example.com","Passwd":"hashed","Salt":"salt"}`},
		{new(EmailAddress), `{"ID":1,"UID":1,"Email":"alice2@example.com","IsActivated":true}`},
		{new(Star), `{"ID":1,"UID":1,"RepoID":1}`},
		{new(IssueUser), `{"ID":1,"UID":1,"IssueID":1,"RepoID":1}`},
		{new(AccessToken), `{"ID":1,"UserID":1,"Name":"legacy","Sha1":"` + sha1 + `"}`},
		{new(Issue), `{"ID":1,"RepoID":1,"Index":1,"Title":"Big","Content":"` + longContentJSON + `"}`},
	}
	for _, row := range rows {
		err := importTable(ctx, db, row.table, strings.NewReader(row.line+"\n"))
		require.NoError(t, err, getTableType(row.table))
	}

	var user User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, "hashed", user.Password)

	var email EmailAddress
	require.NoError(t, db.First(&email, 1).Error)
	assert.Equal(t, int64(1), email.UserID)

	var star Star
	require.NoError(t, db.First(&star, 1).Error)
	assert.Equal(t, int64(1), star.UserID)

	var issueUser IssueUser
	require.NoError(t, db.First(&issueUser, 1).Error)
	assert.Equal(t, int64(1), issueUser.UserID)

	var token AccessToken
	require.NoError(t, db.First(&token, 1).Error)
	assert.Equal(t, cryptox.SHA256(sha1), token.SHA256)

	var issue Issue
	require.NoError(t, db.First(&issue, 1).Error)
	assert.Equal(t, longContent, issue.Content)
}

func setupDBToDump(t *testing.T, db *gorm.DB) {
	vals := []any{
		&Access{
			ID:     1,
			UserID: 1,
			RepoID: 11,
			Mode:   AccessModeRead,
		},
		&Access{
			ID:     2,
			UserID: 2,
			RepoID: 22,
			Mode:   AccessModeWrite,
		},

		&AccessToken{
			UserID:      1,
			Name:        "test1",
			Sha1:        cryptox.SHA1("2910d03d-c0b5-4f71-bad5-c4086e4efae3"),
			SHA256:      cryptox.SHA256(cryptox.SHA1("2910d03d-c0b5-4f71-bad5-c4086e4efae3")),
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486, // 1 hour later
		},
		&AccessToken{
			UserID:      1,
			Name:        "test2",
			Sha1:        cryptox.SHA1("84117e17-7e67-4024-bd04-1c23e6e809d4"),
			SHA256:      cryptox.SHA256(cryptox.SHA1("84117e17-7e67-4024-bd04-1c23e6e809d4")),
			CreatedUnix: 1588568886,
		},
		&AccessToken{
			UserID:      2,
			Name:        "test1",
			Sha1:        cryptox.SHA1("da2775ce-73dd-47ba-b9d2-bbcc346585c4"),
			SHA256:      cryptox.SHA256(cryptox.SHA1("da2775ce-73dd-47ba-b9d2-bbcc346585c4")),
			CreatedUnix: 1588568886,
		},
		&AccessToken{
			UserID:      2,
			Name:        "test2",
			Sha1:        cryptox.SHA256(cryptox.SHA1("1b2dccd1-a262-470f-bb8c-7fc73192e9bb"))[:40],
			SHA256:      cryptox.SHA256(cryptox.SHA1("1b2dccd1-a262-470f-bb8c-7fc73192e9bb")),
			CreatedUnix: 1588568886,
		},

		&Action{
			ID:           1,
			UserID:       1,
			OpType:       ActionCreateBranch,
			ActUserID:    1,
			ActUserName:  "alice",
			RepoID:       1,
			RepoUserName: "alice",
			RepoName:     "example",
			RefName:      "main",
			IsPrivate:    false,
			Content:      `{"Len":1,"Commits":[],"CompareURL":""}`,
			CreatedUnix:  1588568886,
		},
		&Action{
			ID:           2,
			UserID:       1,
			OpType:       ActionCommitRepo,
			ActUserID:    1,
			ActUserName:  "alice",
			RepoID:       1,
			RepoUserName: "alice",
			RepoName:     "example",
			RefName:      "main",
			IsPrivate:    false,
			Content:      `{"Len":1,"Commits":[],"CompareURL":""}`,
			CreatedUnix:  1588568886,
		},
		&Action{
			ID:           3,
			UserID:       1,
			OpType:       ActionDeleteBranch,
			ActUserID:    1,
			ActUserName:  "alice",
			RepoID:       1,
			RepoUserName: "alice",
			RepoName:     "example",
			RefName:      "main",
			IsPrivate:    false,
			CreatedUnix:  1588568886,
		},

		&Attachment{
			ID:          1,
			UUID:        "4b9e1b8c-6c1f-4a3e-9d43-0c5f3c3f8e2a",
			IssueID:     1,
			CommentID:   1,
			ReleaseID:   0,
			Name:        "screenshot.png",
			CreatedUnix: 1588568886,
		},

		&Collaboration{
			ID:     1,
			UserID: 2,
			RepoID: 1,
			Mode:   AccessModeWrite,
		},

		&Comment{
			ID:          1,
			Type:        CommentTypeComment,
			PosterID:    2,
			IssueID:     1,
			Content:     "Looks good to me",
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486,
		},
		&Comment{
			ID:          2,
			Type:        CommentTypeCommitRef,
			PosterID:    1,
			IssueID:     1,
			Content:     `<a href="/alice/example/commit/f3a9c2b">fix the bug</a>`,
			CreatedUnix: 1588568887,
			UpdatedUnix: 1588568887,
			CommitSHA:   "f3a9c2b4f0c9e5c1a2d3e4f5a6b7c8d9e0f1a2b3",
		},

		&DeployKey{
			ID:          1,
			KeyID:       2,
			RepoID:      1,
			Name:        "CI",
			Fingerprint: "SHA256:9W6J1BLrJPGbcvnCoyZKuQPRdOJ7zh4Ss3cn5a1ITqA",
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486,
		},

		&EmailAddress{
			ID:          1,
			UserID:      1,
			Email:       "alice@example.com",
			IsActivated: false,
		},
		&EmailAddress{
			ID:          2,
			UserID:      2,
			Email:       "bob@example.com",
			IsActivated: true,
		},

		&Follow{
			ID:       1,
			UserID:   1,
			FollowID: 2,
		},
		&Follow{
			ID:       2,
			UserID:   2,
			FollowID: 1,
		},

		&HookTask{
			ID:              1,
			RepoID:          1,
			HookID:          1,
			UUID:            "0f9b1a2c-3d4e-4f50-8a6b-7c8d9e0f1a2b",
			Type:            GOGS,
			URL:             "https://example.com/hook",
			Signature:       "5d41402abc4b2a76b9719d911017c592",
			PayloadContent:  `{"ref":"refs/heads/main"}`,
			ContentType:     JSON,
			EventType:       HookEventTypePush,
			IsSSL:           true,
			IsDelivered:     true,
			Delivered:       1588568886000000000,
			IsSucceed:       true,
			RequestContent:  `{"headers":{}}`,
			ResponseContent: `{"status":200}`,
		},

		&Issue{
			ID:           1,
			RepoID:       1,
			Index:        1,
			PosterID:     1,
			Title:        "Something is broken",
			Content:      "Steps to reproduce",
			MilestoneID:  1,
			Priority:     2,
			AssigneeID:   2,
			NumComments:  2,
			DeadlineUnix: 1588655286,
			CreatedUnix:  1588568886,
			UpdatedUnix:  1588572486,
		},
		&Issue{
			ID:          2,
			RepoID:      1,
			Index:       2,
			PosterID:    2,
			Title:       "Fix the bug",
			IsClosed:    true,
			IsPull:      true,
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588568886,
		},

		&IssueLabel{
			ID:      1,
			IssueID: 1,
			LabelID: 1,
		},

		&IssueUser{
			ID:          1,
			UserID:      1,
			IssueID:     1,
			RepoID:      1,
			MilestoneID: 1,
			IsRead:      true,
			IsPoster:    true,
		},
		&IssueUser{
			ID:          2,
			UserID:      2,
			IssueID:     1,
			RepoID:      1,
			MilestoneID: 1,
			IsAssigned:  true,
			IsMentioned: true,
		},

		&Label{
			ID:              1,
			RepoID:          1,
			Name:            "bug",
			Color:           "#ee0701",
			NumIssues:       1,
			NumClosedIssues: 0,
		},

		&LFSObject{
			RepoID:    1,
			OID:       "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
			Size:      100,
			Storage:   lfsx.StorageLocal,
			CreatedAt: time.Unix(1588568886, 0).UTC(),
		},
		&LFSObject{
			RepoID:    2,
			OID:       "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
			Size:      100,
			Storage:   lfsx.StorageLocal,
			CreatedAt: time.Unix(1588568886, 0).UTC(),
		},

		&LoginSource{
			Type:      auth.PAM,
			Name:      "My PAM",
			IsActived: true,
			Provider: pam.NewProvider(&pam.Config{
				ServiceName: "PAM service",
			}),
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486, // 1 hour later
		},
		&LoginSource{
			Type:      auth.GitHub,
			Name:      "GitHub.com",
			IsActived: true,
			Provider: github.NewProvider(&github.Config{
				APIEndpoint: "https://api.github.com",
			}),
			CreatedUnix: 1588568886,
		},

		&Milestone{
			ID:              1,
			RepoID:          1,
			Name:            "v1.0",
			Content:         "The first release",
			NumIssues:       1,
			NumClosedIssues: 0,
			Completeness:    0,
			DeadlineUnix:    1588655286,
			ClosedDateUnix:  0,
		},
		&Milestone{
			ID:              2,
			RepoID:          1,
			Name:            "v0.9",
			IsClosed:        true,
			NumIssues:       3,
			NumClosedIssues: 3,
			Completeness:    100,
			DeadlineUnix:    1588568886,
			ClosedDateUnix:  1588572486,
		},

		&Mirror{
			ID:           1,
			RepoID:       2,
			Interval:     8,
			EnablePrune:  true,
			LastSyncUnix: 1588568886,
			NextSyncUnix: 1588597686,
		},

		&Notice{
			ID:          1,
			Type:        NoticeTypeRepository,
			Description: "This is a notice",
			CreatedUnix: 1588568886,
		},

		&OrgUser{
			ID:       1,
			UID:      1,
			OrgID:    3,
			IsPublic: true,
			IsOwner:  true,
			NumTeams: 1,
		},

		&ProtectBranch{
			ID:                 1,
			RepoID:             1,
			Name:               "main",
			Protected:          true,
			RequirePullRequest: true,
			EnableWhitelist:    true,
			WhitelistUserIDs:   "1,2",
			WhitelistTeamIDs:   "1",
		},

		&ProtectBranchWhitelist{
			ID:              1,
			ProtectBranchID: 1,
			RepoID:          1,
			Name:            "main",
			UserID:          2,
		},

		&PublicKey{
			ID:          1,
			OwnerID:     1,
			Name:        "laptop",
			Fingerprint: "SHA256:ZtRfMl4sIkqP0ofV6QX9QJ0Wy3XFaDpqUVO0xE0lK3U",
			Content:     "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBy4YBnnqqtqN0UfhX6s1nLnNd4C6x5cDaoxqKyQbYxE alice@example.com",
			Mode:        AccessModeWrite,
			Type:        KeyTypeUser,
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486,
		},
		&PublicKey{
			ID:          2,
			OwnerID:     1,
			Name:        "CI",
			Fingerprint: "SHA256:9W6J1BLrJPGbcvnCoyZKuQPRdOJ7zh4Ss3cn5a1ITqA",
			Content:     "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGd0o7l0n9bU8nSl6oTnD3v6Zr0hYz3Jw2nX7c0WbV9P ci@example.com",
			Mode:        AccessModeRead,
			Type:        KeyTypeDeploy,
			CreatedUnix: 1588568886,
		},

		&PullRequest{
			ID:             1,
			Type:           PullRequestTypeGogs,
			Status:         PullRequestStatusMergeable,
			IssueID:        2,
			Index:          2,
			HeadRepoID:     1,
			BaseRepoID:     1,
			HeadUserName:   "alice",
			HeadBranch:     "fix",
			BaseBranch:     "main",
			MergeBase:      "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
			HasMerged:      true,
			MergedCommitID: "f3a9c2b4f0c9e5c1a2d3e4f5a6b7c8d9e0f1a2b3",
			MergerID:       1,
			MergedUnix:     1588572486,
		},

		&Release{
			ID:           1,
			RepoID:       1,
			PublisherID:  1,
			TagName:      "v1.0",
			LowerTagName: "v1.0",
			Target:       "main",
			Title:        "First release",
			Sha1:         "f3a9c2b4f0c9e5c1a2d3e4f5a6b7c8d9e0f1a2b3",
			NumCommits:   42,
			Note:         "Release notes",
			IsPrerelease: true,
			CreatedUnix:  1588568886,
		},

		&Repository{
			ID:                    1,
			OwnerID:               1,
			LowerName:             "example",
			Name:                  "Example",
			Description:           "An example repository",
			Website:               "https://example.com",
			DefaultBranch:         "main",
			Size:                  1024,
			NumWatches:            2,
			NumStars:              1,
			NumForks:              0,
			NumIssues:             1,
			NumPulls:              1,
			NumClosedPulls:        1,
			NumMilestones:         2,
			NumClosedMilestones:   1,
			IsPrivate:             true,
			EnableWiki:            true,
			EnableExternalWiki:    true,
			ExternalWikiURL:       "https://wiki.example.com",
			EnableIssues:          true,
			EnableExternalTracker: true,
			ExternalTrackerURL:    "https://tracker.example.com",
			ExternalTrackerFormat: "https://tracker.example.com/{index}",
			ExternalTrackerStyle:  "numeric",
			EnablePulls:           true,
			PullsAllowRebase:      true,
			CreatedUnix:           1588568886,
			UpdatedUnix:           1588572486,
		},
		&Repository{
			ID:            2,
			OwnerID:       3,
			LowerName:     "mirror",
			Name:          "mirror",
			DefaultBranch: "master",
			IsUnlisted:    true,
			IsMirror:      true,
			IsFork:        true,
			ForkID:        1,
			CreatedUnix:   1588568886,
			UpdatedUnix:   1588568886,
		},

		&Star{
			ID:     1,
			UserID: 2,
			RepoID: 1,
		},

		&Team{
			ID:          1,
			OrgID:       3,
			LowerName:   "owners",
			Name:        "Owners",
			Description: "The owners",
			Authorize:   AccessModeOwner,
			NumRepos:    1,
			NumMembers:  1,
		},

		&TeamRepo{
			ID:     1,
			OrgID:  3,
			TeamID: 1,
			RepoID: 2,
		},

		&TeamUser{
			ID:     1,
			OrgID:  3,
			TeamID: 1,
			UID:    1,
		},

		&TwoFactor{
			ID:          1,
			UserID:      1,
			Secret:      "oR0Mt1ZqRqf2c7G5wHsXbA==",
			CreatedUnix: 1588568886,
		},

		&TwoFactorRecoveryCode{
			ID:     1,
			UserID: 1,
			Code:   "a1b2c-d3e4f",
		},
		&TwoFactorRecoveryCode{
			ID:     2,
			UserID: 1,
			Code:   "f6e5d-c4b3a",
			IsUsed: true,
		},

		&Upload{
			ID:   1,
			UUID: "8c2d0f1e-5a6b-4c7d-9e8f-0a1b2c3d4e5f",
			Name: "notes.txt",
		},

		&User{
			ID:               1,
			LowerName:        "alice",
			Name:             "Alice",
			FullName:         "Alice Anders",
			Email:            "alice@example.com",
			Password:         "6c1f1a9d0e2b3c4d5e6f7a8b9c0d1e2f",
			LoginName:        "alice",
			Type:             UserTypeIndividual,
			Location:         "Earth",
			Website:          "https://alice.example.com",
			Rands:            "abcdefghij",
			Salt:             "klmnopqrst",
			CreatedUnix:      1588568886,
			UpdatedUnix:      1588572486,
			MaxRepoCreation:  -1,
			IsActive:         true,
			IsAdmin:          true,
			AllowGitHook:     true,
			AllowImportLocal: true,
			Avatar:           "1",
			AvatarEmail:      "alice@example.com",
			UseCustomAvatar:  true,
			NumFollowers:     1,
			NumFollowing:     1,
			NumStars:         1,
			NumRepos:         1,
			Description:      "Hello",
		},
		&User{
			ID:              2,
			LowerName:       "bob",
			Name:            "bob",
			Email:           "bob@example.com",
			LoginSource:     1,
			LoginName:       "bob",
			Type:            UserTypeIndividual,
			CreatedUnix:     1588568886,
			UpdatedUnix:     1588568886,
			MaxRepoCreation: 10,
			ProhibitLogin:   true,
			AvatarEmail:     "bob@example.com",
		},
		&User{
			ID:              3,
			LowerName:       "acme",
			Name:            "acme",
			FullName:        "Acme Inc.",
			Type:            UserTypeOrganization,
			CreatedUnix:     1588568886,
			UpdatedUnix:     1588568886,
			MaxRepoCreation: -1,
			IsActive:        true,
			NumRepos:        1,
			NumTeams:        1,
			NumMembers:      1,
		},

		&Watch{
			ID:     1,
			UserID: 1,
			RepoID: 1,
		},
		&Watch{
			ID:     2,
			UserID: 2,
			RepoID: 1,
		},

		&Webhook{
			ID:           1,
			RepoID:       1,
			URL:          "https://example.com/hook",
			ContentType:  JSON,
			Secret:       "s3cr3t",
			Events:       `{"push_only":true,"send_everything":false,"choose_events":false,"events":{}}`,
			IsSSL:        true,
			IsActive:     true,
			HookTaskType: GOGS,
			LastStatus:   HookStatusSucceed,
			CreatedUnix:  1588568886,
			UpdatedUnix:  1588572486,
		},
		&Webhook{
			ID:           2,
			OrgID:        3,
			URL:          "https://hooks.slack.com/services/T/B/X",
			ContentType:  JSON,
			Events:       `{"push_only":false,"send_everything":true,"choose_events":false,"events":{}}`,
			HookTaskType: SLACK,
			Meta:         `{"channel":"#dev","username":"gogs"}`,
			LastStatus:   HookStatusFailed,
			CreatedUnix:  1588568886,
			UpdatedUnix:  1588568886,
		},
	}
	for _, val := range vals {
		err := db.Create(val).Error
		require.NoError(t, err)
	}

	// Store values that hooks or column defaults would replace on create, so that
	// importing has to restore them as they are.
	updates := []struct {
		model  any
		values map[string]any
	}{
		{&Repository{ID: 2}, map[string]any{"enable_wiki": false, "enable_issues": false, "enable_pulls": false}},
		{&Mirror{ID: 1}, map[string]any{"enable_prune": false}},
		{&User{ID: 2}, map[string]any{"max_repo_creation": 0, "created_unix": 0, "updated_unix": 0}},
		{&Milestone{ID: 1}, map[string]any{"deadline_unix": 0}},
		{&Comment{ID: 1}, map[string]any{"created_unix": 0}},
	}
	for _, u := range updates {
		err := db.Model(u.model).UpdateColumns(u.values).Error
		require.NoError(t, err)
	}
}

func dumpTables(t *testing.T, db *gorm.DB) {
	ctx := context.Background()

	for _, table := range Tables {
		tableName := getTableType(table)

		var buf bytes.Buffer
		err := dumpTable(ctx, db, table, &buf)
		if err != nil {
			t.Fatalf("%s: %v", tableName, err)
		}

		golden := filepath.Join("testdata", "backup", tableName+".golden.json")
		testx.AssertGolden(t, golden, testx.Update("TestDumpAndImport"), buf.String())
	}
}

func importTables(t *testing.T, db *gorm.DB) {
	ctx := context.Background()

	for _, table := range Tables {
		tableName := getTableType(table)

		err := func() error {
			golden := filepath.Join("testdata", "backup", tableName+".golden.json")
			f, err := os.Open(golden)
			if err != nil {
				return errors.Wrap(err, "open table file")
			}
			defer func() { _ = f.Close() }()

			return importTable(ctx, db, table, f)
		}()
		if err != nil {
			t.Fatalf("%s: %v", tableName, err)
		}
	}
}
