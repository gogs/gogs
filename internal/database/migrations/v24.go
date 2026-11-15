package migrations

import (
	"gorm.io/gorm"
)

// addRepositoryIsUnlisted adds the column that XORM used to add on startup for
// instances upgraded directly from 0.12.
func addRepositoryIsUnlisted(db *gorm.DB) error {
	type repository struct {
		IsUnlisted bool `gorm:"not null;default:FALSE"`
	}
	if db.Migrator().HasColumn(&repository{}, "IsUnlisted") {
		return errMigrationSkipped
	}
	if db.Name() == "sqlite" {
		// Match the column type of new installations, which GORM declares as numeric.
		return db.Exec("ALTER TABLE `repository` ADD COLUMN `is_unlisted` INTEGER DEFAULT 0 NOT NULL").Error
	}
	return db.Migrator().AddColumn(&repository{}, "IsUnlisted")
}
