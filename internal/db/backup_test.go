// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/lfsutil"
	"gogs.io/gogs/internal/testutil"
)

func TestDumpAndImport(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	if len(Tables) != 6 {
		t.Fatalf("New table has added (want 6 got %d), please add new tests for the table and update this check", len(Tables))
	}

	db := dbtest.NewDB(t, "dumpAndImport", Tables...)
	setupDBToDump(t, db)
	dumpTables(t, db)
	importTables(t, db)

	// Dump and assert golden again to make sure data aren't changed.
	dumpTables(t, db)
}

func setupDBToDump(t *testing.T, db *gorm.DB) {
	vals := []interface{}{
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
			Sha1:        cryptoutil.SHA1("2910d03d-c0b5-4f71-bad5-c4086e4efae3"),
			SHA256:      cryptoutil.SHA256(cryptoutil.SHA1("2910d03d-c0b5-4f71-bad5-c4086e4efae3")),
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486, // 1 hour later
		},
		&AccessToken{
			UserID:      1,
			Name:        "test2",
			Sha1:        cryptoutil.SHA1("84117e17-7e67-4024-bd04-1c23e6e809d4"),
			SHA256:      cryptoutil.SHA256(cryptoutil.SHA1("84117e17-7e67-4024-bd04-1c23e6e809d4")),
			CreatedUnix: 1588568886,
		},
		&AccessToken{
			UserID:      2,
			Name:        "test1",
			Sha1:        cryptoutil.SHA1("da2775ce-73dd-47ba-b9d2-bbcc346585c4"),
			SHA256:      cryptoutil.SHA256(cryptoutil.SHA1("da2775ce-73dd-47ba-b9d2-bbcc346585c4")),
			CreatedUnix: 1588568886,
		},
		&AccessToken{
			UserID:      2,
			Name:        "test2",
			Sha1:        cryptoutil.SHA256(cryptoutil.SHA1("1b2dccd1-a262-470f-bb8c-7fc73192e9bb"))[:40],
			SHA256:      cryptoutil.SHA256(cryptoutil.SHA1("1b2dccd1-a262-470f-bb8c-7fc73192e9bb")),
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

		&LFSObject{
			RepoID:    1,
			OID:       "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
			Size:      100,
			Storage:   lfsutil.StorageLocal,
			CreatedAt: time.Unix(1588568886, 0).UTC(),
		},
		&LFSObject{
			RepoID:    2,
			OID:       "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
			Size:      100,
			Storage:   lfsutil.StorageLocal,
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
	}
	for _, val := range vals {
		err := db.Create(val).Error
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
		testutil.AssertGolden(t, golden, testutil.Update("TestDumpAndImport"), buf.String())
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
