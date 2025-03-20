package migrations

import (
	"fmt"
	gouuid "github.com/satori/go.uuid"
	"gorm.io/gorm"
)

func addUserLocalEmail(db *gorm.DB) error {
	type User struct {
		ID         int64 `gorm:"primaryKey"`
		LocalEmail string
	}

	type UserNotNULL struct {
		ID         int64 `gorm:"primaryKey"`
		LocalEmail string
	}

	if db.Migrator().HasColumn(&User{}, "LocalEmail") {
		return errMigrationSkipped
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Migrator().AddColumn(&User{}, "LocalEmail")
		if err != nil {
			return fmt.Errorf("add column user.local_email error: %s", err.Error())
		}

		const limit = 100
		for {
			var res []User
			err := tx.Table("user").Where("type = ?", 0).Where("local_email = ''").Limit(limit).Find(&res).Error
			if err != nil {
				return fmt.Errorf("query user error: %s", err.Error())
			}

			for _, r := range res {
				r.LocalEmail = gouuid.NewV4().String() + "@fake.localhost"
				err = tx.Save(&r).Error
				if err != nil {
					return fmt.Errorf("save column user.local_email error: %s", err)
				}
			}

			if len(res) < limit {
				break
			}
		}

		err = tx.Migrator().AlterColumn(&User{}, "LocalEmail")
		if err != nil {
			return fmt.Errorf("alter column user.local_email error: %s", err.Error())
		}

		return nil
	})
}
