package migrations

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// repoIDIndexes are the indexes on the columns that deleting a repository
// deletes rows by, which no index led with before.
var repoIDIndexes = []struct{ table, column, name string }{
	{"access", "repo_id", "IDX_access_repo_id"},
	{"watch", "repo_id", "IDX_watch_repo_id"},
	{"pull_request", "base_repo_id", "IDX_pull_request_base_repo_id"},
}

// addRepoIDIndexes indexes the columns that deleting a repository deletes rows
// by. Without them, MySQL locks every row of these tables while deleting a
// repository, which deadlocks with other changes to the same tables.
func addRepoIDIndexes(db *gorm.DB) error {
	added := false
	for _, idx := range repoIDIndexes {
		if db.Migrator().HasIndex(idx.table, idx.name) {
			continue
		}
		err := db.Exec("CREATE INDEX " + quote(db, idx.name) + " ON " + quote(db, idx.table) + " (" + quote(db, idx.column) + ")").Error
		if err != nil {
			return errors.Wrapf(err, "create index %q", idx.name)
		}
		added = true
	}
	if !added {
		return errMigrationSkipped
	}
	return nil
}
