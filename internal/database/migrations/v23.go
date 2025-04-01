package migrations

import (
	"fmt"
	"gorm.io/gorm"
)

func addUserPublicEmail(db *gorm.DB) error {
	type User struct {
		PublicEmail string // 不能使用NOT NULL
	}

	type UserNotNull struct {
		PublicEmail string `xorm:"NOT NULL" gorm:"not null"`
	}

	if db.Migrator().HasColumn(&User{}, "PublicEmail") {
		return errMigrationSkipped
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Migrator().AddColumn(&User{}, "PublicEmail")
		if err != nil {
			return fmt.Errorf("add column user.public_email error: %s", err.Error())
		}

		err = tx.Exec("UPDATE `user` SET `public_email` = `email` WHERE `public_email` = '' AND `type` = 0").Error
		if err != nil {
			return fmt.Errorf("update public_email error: %s", err.Error())
		}

		err = tx.Debug().Migrator().AlterColumn(&UserNotNull{}, "PublicEmail")
		if err != nil {
			return fmt.Errorf("alter column user.public_email error: %s", err.Error())
		}

		return nil
	})
}
