package migrations

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// addMissingAccessTokenUIDIndex gives every database the index on
// "access_token.uid" under the name that new installations use. Gogs 0.12
// created the table with GORM v1, which named the index "idx_access_token_uid",
// and on SQLite, version 20 recreated the table to add the unique "sha256"
// column, which dropped the index.
func addMissingAccessTokenUIDIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex("access_token", "idx_access_token_user_id") {
		return errMigrationSkipped
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasIndex("access_token", "idx_access_token_uid") {
			err := tx.Migrator().DropIndex("access_token", "idx_access_token_uid")
			if err != nil {
				return errors.Wrap(err, "drop index of GORM v1")
			}
		}
		return tx.Exec("CREATE INDEX " + quote(tx, "idx_access_token_user_id") + " ON " + quote(tx, "access_token") + " (" + quote(tx, "uid") + ")").Error
	})
}
