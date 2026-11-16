package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type accessPreV27 struct {
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"uniqueIndex:access_user_repo_unique;not null"`
	RepoID int64 `gorm:"uniqueIndex:access_user_repo_unique;not null"`
	Mode   int   `gorm:"not null"`
}

func (*accessPreV27) TableName() string {
	return "access"
}

type accessV27 struct {
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"uniqueIndex:access_user_repo_unique;not null"`
	RepoID int64 `gorm:"uniqueIndex:access_user_repo_unique;not null;index:idx_access_repo_id"`
	Mode   int   `gorm:"not null"`
}

func (*accessV27) TableName() string {
	return "access"
}

func TestAddIndexToAccessRepoID(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "addIndexToAccessRepoID", new(accessPreV27))
	err := db.Create(&accessPreV27{ID: 1, UserID: 1, RepoID: 2, Mode: 2}).Error
	require.NoError(t, err)
	assert.False(t, db.Migrator().HasIndex(&accessV27{}, "idx_access_repo_id"))

	err = addIndexToAccessRepoID(db)
	require.NoError(t, err)
	assert.True(t, db.Migrator().HasIndex(&accessV27{}, "idx_access_repo_id"))

	var got accessPreV27
	err = db.Where("repo_id = ?", 2).First(&got).Error
	require.NoError(t, err)
	assert.Equal(t, accessPreV27{ID: 1, UserID: 1, RepoID: 2, Mode: 2}, got)

	// Re-run should be skipped
	err = addIndexToAccessRepoID(db)
	require.Equal(t, errMigrationSkipped, err)
}
