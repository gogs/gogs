// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/lfsutil"
	"gogs.io/gogs/internal/testutil"
)

func Test_dumpAndImport(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	if len(Tables) != 3 {
		t.Fatalf("New table has added (want 3 got %d), please add new tests for the table and update this check", len(Tables))
	}

	db := initTestDB(t, "dumpAndImport", Tables...)
	setupDBToDump(t, db)
	dumpTables(t, db)
	importTables(t, db)

	// Dump and assert golden again to make sure data aren't changed.
	dumpTables(t, db)
}

func setupDBToDump(t *testing.T, db *gorm.DB) {
	t.Helper()

	vals := []interface{}{
		&AccessToken{
			UserID:      1,
			Name:        "test1",
			Sha1:        cryptoutil.SHA1("2910d03d-c0b5-4f71-bad5-c4086e4efae3"),
			CreatedUnix: 1588568886,
			UpdatedUnix: 1588572486, // 1 hour later
		},
		&AccessToken{
			UserID:      1,
			Name:        "test2",
			Sha1:        cryptoutil.SHA1("84117e17-7e67-4024-bd04-1c23e6e809d4"),
			CreatedUnix: 1588568886,
		},
		&AccessToken{
			UserID:      2,
			Name:        "test1",
			Sha1:        cryptoutil.SHA1("da2775ce-73dd-47ba-b9d2-bbcc346585c4"),
			CreatedUnix: 1588568886,
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
		if err != nil {
			t.Fatal(err)
		}
	}
}

func dumpTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	for _, table := range Tables {
		tableName := getTableType(table)

		var buf bytes.Buffer
		err := dumpTable(db, table, &buf)
		if err != nil {
			t.Fatalf("%s: %v", tableName, err)
		}

		golden := filepath.Join("testdata", "backup", tableName+".golden.json")
		testutil.AssertGolden(t, golden, testutil.Update("Test_dumpAndImport"), buf.String())
	}
}

func importTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	for _, table := range Tables {
		tableName := getTableType(table)

		err := func() error {
			golden := filepath.Join("testdata", "backup", tableName+".golden.json")
			f, err := os.Open(golden)
			if err != nil {
				return errors.Wrap(err, "open table file")
			}
			defer func() { _ = f.Close() }()

			return importTable(db, table, f)
		}()
		if err != nil {
			t.Fatalf("%s: %v", tableName, err)
		}
	}
}
