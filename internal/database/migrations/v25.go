package migrations

import (
	"fmt"
	"gorm.io/gorm"
)

func insertUserPrimaryEmail(db *gorm.DB) error {
	type EmailAddress struct {
		ID          int64  `gorm:"primaryKey"`
		UserID      int64  `xorm:"uid INDEX NOT NULL" gorm:"column:uid;index;uniqueIndex:email_address_user_email_unique;not null"`
		Email       string `xorm:"UNIQUE NOT NULL" gorm:"uniqueIndex:email_address_user_email_unique;not null;size:254"`
		IsActivated bool   `gorm:"not null;default:FALSE"`
	}

	type User struct {
		ID       int64  `gorm:"primaryKey"`
		Email    string `xorm:"NOT NULL" gorm:"not null"`
		IsActive bool   // Activate primary email
	}

	return db.Transaction(func(tx *gorm.DB) error {
		const limit = 100
		var offset = 0
		for {
			var res []User
			err := tx.Table("user").Where("type = ?", 0).Offset(offset).Limit(limit).Find(&res).Error
			if err != nil {
				return fmt.Errorf("query user error: %s", err.Error())
			}

			for _, r := range res {
				record := &EmailAddress{
					UserID:      r.ID,
					Email:       r.Email,
					IsActivated: r.IsActive,
				}
				err := tx.Table("email_address").Where("uid = ? AND email = ?", record.UserID, record.Email).FirstOrCreate(record).Error
				if err != nil {
					return fmt.Errorf("insert email error: %s", err.Error())
				}
			}

			if len(res) < limit {
				break
			} else {
				offset += len(res)
			}
		}

		return nil
	})
}
