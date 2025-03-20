package migrations

import (
	"gorm.io/gorm"
)

func addUserPublicEmail(db *gorm.DB) error {
	type User struct {
		PublicEmail string `xorm:"NOT NULL" gorm:"not null"`
	}

	if db.Migrator().HasColumn(&User{}, "PublicEmail") {
		return errMigrationSkipped
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Migrator().AddColumn(&User{}, "PublicEmail")
		if err != nil {
			return err
		}

		err = tx.Exec("UPDATE `user` SET `public_email` = `email` WHERE `public_email` = '' AND `type` = 0").Error
		if err != nil {
			return err
		}

		return nil
	})
}
