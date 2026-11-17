package database

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
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

	const wantTables = 30
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

		&Passkey{
			ID:     1,
			UserID: 1,
			Name:   "Laptop",
			Credential: webauthn.Credential{
				ID:              []byte("credential-1"),
				PublicKey:       []byte("public-key-1"),
				AttestationType: "none",
				Transport:       []protocol.AuthenticatorTransport{protocol.Internal, protocol.Hybrid},
				Flags:           webauthn.CredentialFlags{UserPresent: true, UserVerified: true, BackupEligible: true, BackupState: true},
				Authenticator:   webauthn.Authenticator{SignCount: 5},
			},
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486, // 1 hour later
		},

		&ProtectBranch{
			ID:                 1,
			RepoID:             1,
			Name:               "main",
			Protected:          true,
			RequirePullRequest: true,
			EnableAllowlist:    true,
			AllowlistUserIDs:   "1,2",
			AllowlistTeamIDs:   "1",
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
			Description: "Owners of the organization",
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
		{&User{ID: 2}, map[string]any{"max_repo_creation": 0, "created_unix": 0, "updated_unix": 0}},
		{&Milestone{ID: 1}, map[string]any{"deadline_unix": 0}},
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
