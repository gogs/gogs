// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/lfsutil"
)

func TestLFS(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &LFSStore{
		db: newTestDB(t, "LFSStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *LFSStore)
	}{
		{"CreateObject", lfsCreateObject},
		{"GetObjectByOID", lfsGetObjectByOID},
		{"GetObjectsByOIDs", lfsGetObjectsByOIDs},
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

func lfsCreateObject(t *testing.T, ctx context.Context, s *LFSStore) {
	// Create first LFS object
	repoID := int64(1)
	oid := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	err := s.CreateObject(ctx, repoID, oid, 12, lfsutil.StorageLocal)
	require.NoError(t, err)

	// Get it back and check the CreatedAt field
	object, err := s.GetObjectByOID(ctx, repoID, oid)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), object.CreatedAt.UTC().Format(time.RFC3339))

	// Try to create second LFS object with same oid should fail
	err = s.CreateObject(ctx, repoID, oid, 12, lfsutil.StorageLocal)
	assert.Error(t, err)
}

func lfsGetObjectByOID(t *testing.T, ctx context.Context, s *LFSStore) {
	// Create a LFS object
	repoID := int64(1)
	oid := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	err := s.CreateObject(ctx, repoID, oid, 12, lfsutil.StorageLocal)
	require.NoError(t, err)

	// We should be able to get it back
	_, err = s.GetObjectByOID(ctx, repoID, oid)
	require.NoError(t, err)

	// Try to get a non-existent object
	_, err = s.GetObjectByOID(ctx, repoID, "bad_oid")
	expErr := ErrLFSObjectNotExist{args: errutil.Args{"repoID": repoID, "oid": lfsutil.OID("bad_oid")}}
	assert.Equal(t, expErr, err)
}

func lfsGetObjectsByOIDs(t *testing.T, ctx context.Context, s *LFSStore) {
	// Create two LFS objects
	repoID := int64(1)
	oid1 := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	oid2 := lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64g")
	err := s.CreateObject(ctx, repoID, oid1, 12, lfsutil.StorageLocal)
	require.NoError(t, err)
	err = s.CreateObject(ctx, repoID, oid2, 12, lfsutil.StorageLocal)
	require.NoError(t, err)

	// We should be able to get them back and ignore non-existent ones
	objects, err := s.GetObjectsByOIDs(ctx, repoID, oid1, oid2, "bad_oid")
	require.NoError(t, err)
	assert.Equal(t, 2, len(objects), "number of objects")

	assert.Equal(t, repoID, objects[0].RepoID)
	assert.Equal(t, oid1, objects[0].OID)

	assert.Equal(t, repoID, objects[1].RepoID)
	assert.Equal(t, oid2, objects[1].OID)
}
