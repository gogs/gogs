package migrations

import (
	"gorm.io/gorm"
)

func addIndexToActionUserID(db *gorm.DB) error {
	type action struct {
		UserID string `gorm:"index"`
	}
	if db.Migrator().HasIndex(&action{}, "UserID") {
		return errMigrationSkipped
	}
	return db.Migrator().CreateIndex(&action{}, "UserID")
}
