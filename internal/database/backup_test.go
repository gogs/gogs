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

	const wantTables = 29
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

		&Attachment{
			ID:          1,
			UUID:        "7d2b8c1e-0f3a-4b5c-9d6e-1a2b3c4d5e6f",
			IssueID:     1,
			CommentID:   2,
			UploaderID:  1,
			Name:        "screenshot.png",
			CreatedUnix: 1588568886,
		},
		&Attachment{
			ID:          2,
			UUID:        "9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b",
			ReleaseID:   1,
			UploaderID:  2,
			Name:        "gogs.zip",
			CreatedUnix: 1588572486,
		},

		&Collaboration{
			ID:     1,
			UserID: 2,
			RepoID: 1,
			Mode:   AccessModeWrite,
		},
		&Collaboration{
			ID:     2,
			UserID: 3,
			RepoID: 1,
			Mode:   AccessModeRead,
		},

		&Comment{
			ID:          1,
			Type:        CommentTypeComment,
			PosterID:    2,
			IssueID:     1,
			Content:     "Me too.",
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486,
		},
		&Comment{
			ID:          2,
			Type:        CommentTypeCommitRef,
			PosterID:    1,
			IssueID:     1,
			Content:     "Fix the crash",
			CommitSHA:   "a4b4f7a8a2e2c0b4f2e9d1c3b5a7f9e1d3c5b7a9",
			CreatedUnix: 1588572486,
			UpdatedUnix: 1588572486,
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
			Title:        "Crash on start",
			Content:      "It **crashes**.",
			MilestoneID:  1,
			Priority:     1,
			AssigneeID:   2,
			NumComments:  2,
			DeadlineUnix: 1588568886,
			CreatedUnix:  1588568886,
			UpdatedUnix:  1588572486,
		},
		&Issue{
			ID:          2,
			RepoID:      1,
			Index:       2,
			PosterID:    2,
			Title:       "Fix the crash",
			IsClosed:    true,
			IsPull:      true,
			CreatedUnix: 1588572486,
			UpdatedUnix: 1588572486,
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
			IsAssigned:  true,
			IsMentioned: true,
			IsClosed:    true,
		},

		&Label{
			ID:              1,
			RepoID:          1,
			Name:            "bug",
			Color:           "#ee0701",
			NumIssues:       3,
			NumClosedIssues: 1,
		},
		&Label{
			ID:     2,
			RepoID: 1,
			Name:   "Help wanted",
			Color:  "#128a0c",
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
			Content:         "The first milestone.",
			NumIssues:       4,
			NumClosedIssues: 1,
			Completeness:    25,
			DeadlineUnix:    1588568886,
		},
		&Milestone{
			ID:              2,
			RepoID:          1,
			Name:            "v0.9",
			IsClosed:        true,
			NumIssues:       2,
			NumClosedIssues: 2,
			Completeness:    100,
			DeadlineUnix:    253402300799,
			ClosedDateUnix:  1588572486,
		},

		&Mirror{
			ID:           1,
			RepoID:       1,
			Interval:     8,
			EnablePrune:  true,
			LastSyncUnix: 1588568886,
			NextSyncUnix: 1588597686,
		},
		&Mirror{
			ID:           2,
			RepoID:       2,
			Interval:     24,
			LastSyncUnix: 1588568886,
			NextSyncUnix: 1588655286,
		},

		&Notice{
			ID:          1,
			Type:        NoticeTypeRepository,
			Description: "This is a notice",
			CreatedUnix: 1588568886,
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
			HeadRepoID:     2,
			BaseRepoID:     1,
			HeadUserName:   "bob",
			HeadBranch:     "fix",
			BaseBranch:     "main",
			MergeBase:      "e180bd23085f8023cbe3625b21c3c387534c3b0d",
			HasMerged:      true,
			MergedCommitID: "a4b4f7a8a2e2c0b4f2e9d1c3b5a7f9e1d3c5b7a9",
			MergerID:       1,
			MergedUnix:     1588572486,
		},

		&Release{
			ID:           1,
			RepoID:       1,
			PublisherID:  1,
			TagName:      "v1.0.0",
			LowerTagName: "v1.0.0",
			Target:       "main",
			Title:        "First release",
			Sha1:         "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
			NumCommits:   12,
			Note:         "Notes of the first release.",
			IsPrerelease: true,
			CreatedUnix:  1588568886,
		},
		&Release{
			ID:           2,
			RepoID:       1,
			PublisherID:  2,
			TagName:      "V2.0.0",
			LowerTagName: "v2.0.0",
			Target:       "main",
			IsDraft:      true,
			CreatedUnix:  1588572486,
		},

		&Star{
			ID:     1,
			UserID: 2,
			RepoID: 1,
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
	// Creating leaves out a false EnablePrune for the column default to apply.
	err := db.Model(new(Mirror)).Where("id = ?", 2).Update("enable_prune", false).Error
	require.NoError(t, err)
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
