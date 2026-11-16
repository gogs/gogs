package migrations

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type lfsForksRepository struct {
	ID     int64 `gorm:"primaryKey"`
	ForkID int64
}

func (*lfsForksRepository) TableName() string {
	return "repository"
}

type lfsForksLFSObject struct {
	RepoID    int64     `gorm:"primaryKey;auto_increment:false"`
	OID       string    `gorm:"primaryKey;column:oid;size:64"`
	Size      int64     `gorm:"not null"`
	Storage   string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (*lfsForksLFSObject) TableName() string {
	return "lfs_object"
}

func TestCopyLFSObjectsToForks(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "copyLFSObjectsToForks", new(lfsForksRepository), new(lfsForksLFSObject))
	// Repository 2 is a fork of 1 with an object of its own, and 4 is a fork of
	// 2, created before 2 and so with a lower ID. Repository 3 is a fork of a
	// deleted repository, and 5 is unrelated.
	require.NoError(t, db.Create([]*lfsForksRepository{{ID: 1}, {ID: 2, ForkID: 1}, {ID: 3}, {ID: 4, ForkID: 2}, {ID: 5}}).Error)
	now := time.Now()
	require.NoError(t, db.Create([]*lfsForksLFSObject{
		{RepoID: 1, OID: "aa", Size: 1, Storage: "local", CreatedAt: now},
		{RepoID: 1, OID: "bb", Size: 2, Storage: "local", CreatedAt: now},
		{RepoID: 2, OID: "bb", Size: 2, Storage: "local", CreatedAt: now},
		{RepoID: 2, OID: "cc", Size: 3, Storage: "local", CreatedAt: now},
		{RepoID: 5, OID: "dd", Size: 4, Storage: "local", CreatedAt: now},
	}).Error)

	want := map[int64][]string{
		1: {"aa", "bb"},
		2: {"aa", "bb", "cc"},
		4: {"aa", "bb", "cc"},
		5: {"dd"},
	}
	for range 2 {
		require.NoError(t, copyLFSObjectsToForks(db))

		var objects []*lfsForksLFSObject
		require.NoError(t, db.Order("repo_id, oid").Find(&objects).Error)
		got := make(map[int64][]string)
		for _, object := range objects {
			got[object.RepoID] = append(got[object.RepoID], object.OID)
		}
		assert.Equal(t, want, got)
	}
}
