// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/dbutil"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/lfsutil"
)

func TestLFS(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{&LFSObject{}}
	db, cleanup := newTestDB(t, "lfs", tables...)
	store := NewLFSStore(db)

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *lfs)
	}{
		{"CreateObject", testLFSCreateObject},
		{"GetObjectByOID", testLFSGetObjectByOID},
		{"GetObjectsByOIDs", testLFSGetObjectsByOIDs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := cleanup()
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, context.Background(), store.(*lfs))
		})
	}
}

func testLFSCreateObject(t *testing.T, ctx context.Context, db *lfs) {
	// Create first LFS object
	repoID := int64(1)
	oid := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	err := db.CreateObject(ctx, repoID, oid, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}

	// Try create second LFS object with same oid should fail
	err = db.CreateObject(ctx, repoID, oid, 12, lfsutil.StorageLocal)
	assert.Error(t, err)
}

func testLFSGetObjectByOID(t *testing.T, ctx context.Context, db *lfs) {
	// Create a LFS object
	repoID := int64(1)
	oid := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	err := db.CreateObject(ctx, repoID, oid, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	got, err := db.GetObjectByOID(ctx, repoID, oid)
	if err != nil {
		t.Fatal(err)
	}
	got.CreatedAt = dbutil.RecreateTime(got.CreatedAt)
	want := &LFSObject{
		RepoID:    repoID,
		OID:       oid,
		Size:      12,
		Storage:   lfsutil.StorageLocal,
		CreatedAt: db.NowFunc(),
	}
	assert.Equal(t, want, got)

	// Try to get a non-existent object
	_, err = db.GetObjectByOID(ctx, repoID, "bad_oid")
	expErr := ErrLFSObjectNotExist{args: errutil.Args{"repoID": repoID, "oid": lfsutil.OID("bad_oid")}}
	assert.Equal(t, expErr, err)
}

func testLFSGetObjectsByOIDs(t *testing.T, ctx context.Context, db *lfs) {
	// Create two LFS objects
	repoID := int64(1)
	oid1 := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	oid2 := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64g")
	err := db.CreateObject(ctx, repoID, oid1, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}
	err = db.CreateObject(ctx, repoID, oid2, 12, lfsutil.StorageLocal)
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get them back and ignore non-existent ones
	objects, err := db.GetObjectsByOIDs(ctx, repoID, oid1, oid2, "bad_oid")
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range objects {
		obj.CreatedAt = dbutil.RecreateTime(obj.CreatedAt)
	}

	want := []*LFSObject{
		{
			RepoID:    repoID,
			OID:       oid1,
			Size:      12,
			Storage:   lfsutil.StorageLocal,
			CreatedAt: db.NowFunc(),
		}, {
			RepoID:    repoID,
			OID:       oid2,
			Size:      12,
			Storage:   lfsutil.StorageLocal,
			CreatedAt: db.NowFunc(),
		},
	}
	assert.Equal(t, want, objects)
}
