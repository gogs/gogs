package migrations

import (
	"gorm.io/gorm"
)

// addIndexToAccessRepoID indexes access.repo_id. Recalculating the accesses
// of a repository deletes its rows by repo_id, which without the index scans
// and locks the whole table.
func addIndexToAccessRepoID(db *gorm.DB) error {
	type access struct {
		RepoID int64 `gorm:"index"`
	}
	if db.Migrator().HasIndex(&access{}, "RepoID") {
		return errMigrationSkipped
	}
	return db.Migrator().CreateIndex(&access{}, "RepoID")
}
