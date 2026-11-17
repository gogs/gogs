package migrations

import (
	"gorm.io/gorm"
)

// addRepositoryPullsAllowMergeAndSquash adds the columns that XORM used to add
// on startup since squash merging was added. Existing repositories keep
// allowing merge commits.
func addRepositoryPullsAllowMergeAndSquash(db *gorm.DB) error {
	return addMissingColumns(db, "repository", map[string]map[string]string{
		"pulls_allow_merge": {
			"sqlite":   "INTEGER DEFAULT 1 NOT NULL",
			"postgres": "BOOL DEFAULT true NOT NULL",
			"mysql":    "TINYINT(1) DEFAULT 1 NOT NULL",
		},
		"pulls_allow_squash": {
			"sqlite":   "INTEGER DEFAULT 0 NOT NULL",
			"postgres": "BOOL DEFAULT false NOT NULL",
			"mysql":    "TINYINT(1) DEFAULT 0 NOT NULL",
		},
	})
}
