// Copyright 2023 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbtest"
)

func TestNotice_BeforeCreate(t *testing.T) {
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
		notice := &Notice{
			CreatedUnix: 1,
		}
		_ = notice.BeforeCreate(db)
		assert.Equal(t, int64(1), notice.CreatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		notice := &Notice{}
		_ = notice.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), notice.CreatedUnix)
	})
}

func TestNotice_AfterFind(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	notice := &Notice{
		CreatedUnix: now.Unix(),
	}
	_ = notice.AfterFind(db)
	assert.Equal(t, notice.CreatedUnix, notice.Created.Unix())
}

func TestNotices(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	tables := []any{new(Notice)}
	db := &notices{
		DB: dbtest.NewDB(t, "notices", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *notices)
	}{
		{"Create", noticesCreate},
		{"DeleteByIDs", noticesDeleteByIDs},
		{"DeleteAll", noticesDeleteAll},
		{"List", noticesList},
		{"Count", noticesCount},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				require.NoError(t, err)
			})
			tc.test(t, ctx, db)
		})
		if t.Failed() {
			break
		}
	}
}

func noticesCreate(t *testing.T, ctx context.Context, db *notices) {
	err := db.Create(ctx, NoticeTypeRepository, "test")
	require.NoError(t, err)

	count := db.Count(ctx)
	assert.Equal(t, int64(1), count)
}

func noticesDeleteByIDs(t *testing.T, ctx context.Context, db *notices) {
	err := db.Create(ctx, NoticeTypeRepository, "test")
	require.NoError(t, err)

	notices, err := db.List(ctx, 1, 10)
	require.NoError(t, err)
	ids := make([]int64, 0, len(notices))
	for _, notice := range notices {
		ids = append(ids, notice.ID)
	}

	// Non-existing IDs should be ignored
	ids = append(ids, 404)
	err = db.DeleteByIDs(ctx, ids...)
	require.NoError(t, err)

	count := db.Count(ctx)
	assert.Equal(t, int64(0), count)
}

func noticesDeleteAll(t *testing.T, ctx context.Context, db *notices) {
	err := db.Create(ctx, NoticeTypeRepository, "test")
	require.NoError(t, err)

	err = db.DeleteAll(ctx)
	require.NoError(t, err)

	count := db.Count(ctx)
	assert.Equal(t, int64(0), count)
}

func noticesList(t *testing.T, ctx context.Context, db *notices) {
	err := db.Create(ctx, NoticeTypeRepository, "test 1")
	require.NoError(t, err)
	err = db.Create(ctx, NoticeTypeRepository, "test 2")
	require.NoError(t, err)

	got1, err := db.List(ctx, 1, 1)
	require.NoError(t, err)
	require.Len(t, got1, 1)

	got2, err := db.List(ctx, 2, 1)
	require.NoError(t, err)
	require.Len(t, got2, 1)
	assert.True(t, got1[0].ID > got2[0].ID)

	got, err := db.List(ctx, 1, 3)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func noticesCount(t *testing.T, ctx context.Context, db *notices) {
	count := db.Count(ctx)
	assert.Equal(t, int64(0), count)

	err := db.Create(ctx, NoticeTypeRepository, "test")
	require.NoError(t, err)

	count = db.Count(ctx)
	assert.Equal(t, int64(1), count)
}
