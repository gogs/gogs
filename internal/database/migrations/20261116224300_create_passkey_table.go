package migrations

import (
	"gorm.io/gorm"
)

// createPasskeyTable creates the table that GORM used to create on startup
// since passkeys were added.
func createPasskeyTable(db *gorm.DB) error {
	type passkey struct {
		ID          int64  `gorm:"primarykey"`
		UserID      int64  `gorm:"index;not null"`
		Name        string `gorm:"not null"`
		Credential  string `gorm:"type:TEXT;not null"`
		CreatedUnix int64
		UpdatedUnix int64
	}
	if db.Migrator().HasTable(&passkey{}) {
		return errMigrationSkipped
	}
	return db.Migrator().CreateTable(&passkey{})
}
