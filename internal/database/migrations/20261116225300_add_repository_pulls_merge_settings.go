package migrations

import (
	"gorm.io/gorm"
)

// addRepositoryPullsMergeSettings adds the columns that XORM used to add on
// startup since repositories choose whether to delete head branches after
// merging and how merge and squash commit messages start.
func addRepositoryPullsMergeSettings(db *gorm.DB) error {
	return addMissingColumns(db, "repository", map[string]map[string]string{
		"pulls_delete_branch": {
			"sqlite":   "INTEGER DEFAULT 0 NOT NULL",
			"postgres": "BOOL DEFAULT false NOT NULL",
			"mysql":    "TINYINT(1) DEFAULT 0 NOT NULL",
		},
		"pulls_merge_message": {
			"sqlite":   "TEXT DEFAULT '' NOT NULL",
			"postgres": "VARCHAR(255) DEFAULT '' NOT NULL",
			"mysql":    "VARCHAR(255) DEFAULT '' NOT NULL",
		},
		"pulls_squash_message": {
			"sqlite":   "TEXT DEFAULT '' NOT NULL",
			"postgres": "VARCHAR(255) DEFAULT '' NOT NULL",
			"mysql":    "VARCHAR(255) DEFAULT '' NOT NULL",
		},
	})
}
