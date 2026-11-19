package migrations

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// addMissingAccessTokenUIDIndex gives every database the index on
// "access_token.uid" under the name that new installations use. Databases that
// started on Gogs 0.11 have it as XORM named it, "IDX_access_token_uid", and
// ones that started on 0.12 as GORM v1 named it, "idx_access_token_uid". On
// SQLite, version 20 recreated the table to add the unique "sha256" column,
// which dropped the index.
func addMissingAccessTokenUIDIndex(db *gorm.DB) error {
	const name = "idx_access_token_user_id"
	if db.Migrator().HasIndex("access_token", name) {
		return errMigrationSkipped
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// MySQL matches index names case-insensitively, so it finds the first name
		// under either spelling and the second no longer exists after dropping it.
		for _, legacyName := range []string{"idx_access_token_uid", "IDX_access_token_uid"} {
			if !tx.Migrator().HasIndex("access_token", legacyName) {
				continue
			}
			err := tx.Migrator().DropIndex("access_token", legacyName)
			if err != nil {
				return errors.Wrapf(err, "drop index %q", legacyName)
			}
		}
		return tx.Exec("CREATE INDEX " + quote(tx, name) + " ON " + quote(tx, "access_token") + " (" + quote(tx, "uid") + ")").Error
	})
}
