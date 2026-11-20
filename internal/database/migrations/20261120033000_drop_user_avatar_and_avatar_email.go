package migrations

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// dropUserAvatarAndAvatarEmail drops the columns that stored the email whose
// Gravatar a user showed and its hash. Avatars are no longer looked up by email.
func dropUserAvatarAndAvatarEmail(db *gorm.DB) error {
	dropped := false
	for _, column := range []string{"avatar", "avatar_email"} {
		if !db.Migrator().HasColumn("user", column) {
			continue
		}
		if err := db.Exec("ALTER TABLE " + quote(db, "user") + " DROP COLUMN " + quote(db, column)).Error; err != nil {
			return errors.Wrapf(err, "drop column %q", column)
		}
		dropped = true
	}
	if !dropped {
		return errMigrationSkipped
	}
	return nil
}
