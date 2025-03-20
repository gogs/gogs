package migrations

import (
	"github.com/pkg/errors"
	gouuid "github.com/satori/go.uuid"
	"gorm.io/gorm"
)

func addUserLocalEmail(db *gorm.DB) error {
	type User struct {
		ID         int64  `gorm:"primaryKey"`
		LocalEmail string `xorm:"NOT NULL" gorm:"not null"`
	}

	if db.Migrator().HasColumn(&User{}, "LocalEmail") {
		return errMigrationSkipped
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Migrator().AddColumn(&User{}, "LocalEmail")
		if err != nil {
			return err
		}

		const limit = 100
		for {
			var res []User
			err := tx.Table("user").Where("type = ?", 0).Where("local_email = ''").Limit(limit).Find(&res).Error
			if err != nil {
				return errors.Wrap(err, "query user")
			}

			for _, r := range res {
				r.LocalEmail = gouuid.NewV4().String() + "@fake.localhost"
				err = tx.Save(&r).Error
				if err != nil {
					return errors.Wrap(err, "save user")
				}
			}

			if len(res) < limit {
				break
			}
		}

		return nil
	})
}
