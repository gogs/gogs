package migrations

import (
	"gorm.io/gorm"
)

// addMissingAccessTokenUIDIndex creates the index on "access_token.uid" again
// where version 20 lost it. On SQLite, that migration recreated the table to add
// the unique "sha256" column, which dropped the indexes of the table.
func addMissingAccessTokenUIDIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex("access_token", "idx_access_token_user_id") {
		return errMigrationSkipped
	}
	return db.Exec("CREATE INDEX " + quote(db, "idx_access_token_user_id") + " ON " + quote(db, "access_token") + " (" + quote(db, "uid") + ")").Error
}
