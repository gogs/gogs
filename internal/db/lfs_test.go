// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/lfsutil"
)

func Test_lfs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(LFSObject)}
	db := &lfs{
		DB: initTestDB(t, "lfs", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *lfs)
	}{
		{"CreateObject", test_lfs_CreateObject},
		{"GetObjectByOID", test_lfs_GetObjectByOID},
		{"GetObjectsByOIDs", test_lfs_GetObjectsByOIDs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, db)
		})
	}
}

func test_lfs_CreateObject(t *testing.T, db *lfs) {
	// Create first LFS object
	repoID := int64(1)
	oid := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	err := db.CreateObject(repoID, oid, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}

	// Get it back and check the CreatedAt field
	object, err := db.GetObjectByOID(repoID, oid)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), object.CreatedAt.Format(time.RFC3339))

	// Try create second LFS object with same oid should fail
	err = db.CreateObject(repoID, oid, 12, lfsutil.StorageLocal)
	assert.Error(t, err)
}

func test_lfs_GetObjectByOID(t *testing.T, db *lfs) {
	// Create a LFS object
	repoID := int64(1)
	oid := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	err := db.CreateObject(repoID, oid, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	_, err = db.GetObjectByOID(repoID, oid)
	if err != nil {
		t.Fatal(err)
	}

	// Try to get a non-existent object
	_, err = db.GetObjectByOID(repoID, "bad_oid")
	expErr := ErrLFSObjectNotExist{args: errutil.Args{"repoID": repoID, "oid": lfsutil.OID("bad_oid")}}
	assert.Equal(t, expErr, err)
}

func test_lfs_GetObjectsByOIDs(t *testing.T, db *lfs) {
	// Create two LFS objects
	repoID := int64(1)
	oid1 := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	oid2 := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64g")
	err := db.CreateObject(repoID, oid1, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}
	err = db.CreateObject(repoID, oid2, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get them back and ignore non-existent ones
	objects, err := db.GetObjectsByOIDs(repoID, oid1, oid2, "bad_oid")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(objects), "number of objects")

	assert.Equal(t, repoID, objects[0].RepoID)
	assert.Equal(t, oid1, objects[0].OID)

	assert.Equal(t, repoID, objects[1].RepoID)
	assert.Equal(t, oid2, objects[1].OID)
}
