package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type protectBranchWhitelist struct {
	ID              int64 `gorm:"primaryKey"`
	ProtectBranchID int64
	RepoID          int64
	Name            string
	UserID          int64
}

func TestDropProtectBranchWhitelistTable(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "dropProtectBranchWhitelistTable", new(protectBranchWhitelist))
	err := db.Create(&protectBranchWhitelist{ID: 1, ProtectBranchID: 1, RepoID: 1, Name: "main", UserID: 1}).Error
	require.NoError(t, err)

	err = dropProtectBranchWhitelistTable(db)
	require.NoError(t, err)
	assert.False(t, db.Migrator().HasTable("protect_branch_whitelist"))

	// Re-run should be skipped
	err = dropProtectBranchWhitelistTable(db)
	require.Equal(t, errMigrationSkipped, err)
}
