package migrations

import (
	"slices"

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
	indexes, err := db.Migrator().GetIndexes("access_token")
	if err != nil {
		return errors.Wrap(err, "get indexes")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, index := range indexes {
			if !slices.Equal(index.Columns(), []string{"uid"}) {
				continue
			}
			err := tx.Migrator().DropIndex("access_token", index.Name())
			if err != nil {
				return errors.Wrapf(err, "drop index %q", index.Name())
			}
		}
		return tx.Exec("CREATE INDEX " + quote(tx, name) + " ON " + quote(tx, "access_token") + " (" + quote(tx, "uid") + ")").Error
	})
}
