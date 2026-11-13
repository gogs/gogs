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
	return db.Migrator().AddColumn(&repository{}, "IsUnlisted")
}
