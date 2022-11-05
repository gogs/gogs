// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"strings"

	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/userutil"
)

// EmailAddresses is the list of all email addresses of a user. Can contain the
// primary email address, but is not obligatory.
type EmailAddress struct {
	ID          int64  `gorm:"primaryKey"`
	UserID      int64  `xorm:"uid INDEX NOT NULL" gorm:"column:uid;index;not null"`
	Email       string `xorm:"UNIQUE NOT NULL" gorm:"unique;not null"`
	IsActivated bool   `gorm:"not null;default:FALSE"`
	IsPrimary   bool   `xorm:"-" gorm:"-" json:"-"`
}

// GetEmailAddresses returns all email addresses belongs to given user.
func GetEmailAddresses(uid int64) ([]*EmailAddress, error) {
	emails := make([]*EmailAddress, 0, 5)
	if err := x.Where("uid=?", uid).Find(&emails); err != nil {
		return nil, err
	}

	u, err := Users.GetByID(context.TODO(), uid)
	if err != nil {
		return nil, err
	}

	isPrimaryFound := false
	for _, email := range emails {
		if email.Email == u.Email {
			isPrimaryFound = true
			email.IsPrimary = true
		} else {
			email.IsPrimary = false
		}
	}

	// We always want the primary email address displayed, even if it's not in
	// the emailaddress table (yet).
	if !isPrimaryFound {
		emails = append(emails, &EmailAddress{
			Email:       u.Email,
			IsActivated: true,
			IsPrimary:   true,
		})
	}
	return emails, nil
}

func isEmailUsed(e Engine, email string) (bool, error) {
	if email == "" {
		return true, nil
	}

	has, err := e.Get(&EmailAddress{Email: email})
	if err != nil {
		return false, err
	} else if has {
		return true, nil
	}

	// We need to check primary email of users as well.
	return e.Where("type=?", UserTypeIndividual).And("email=?", email).Get(new(User))
}

// IsEmailUsed returns true if the email has been used.
func IsEmailUsed(email string) (bool, error) {
	return isEmailUsed(x, email)
}

func addEmailAddress(e Engine, email *EmailAddress) error {
	email.Email = strings.ToLower(strings.TrimSpace(email.Email))
	used, err := isEmailUsed(e, email.Email)
	if err != nil {
		return err
	} else if used {
		return ErrEmailAlreadyUsed{args: errutil.Args{"email": email.Email}}
	}

	_, err = e.Insert(email)
	return err
}

func AddEmailAddress(email *EmailAddress) error {
	return addEmailAddress(x, email)
}

func AddEmailAddresses(emails []*EmailAddress) error {
	if len(emails) == 0 {
		return nil
	}

	// Check if any of them has been used
	for i := range emails {
		emails[i].Email = strings.ToLower(strings.TrimSpace(emails[i].Email))
		used, err := IsEmailUsed(emails[i].Email)
		if err != nil {
			return err
		} else if used {
			return ErrEmailAlreadyUsed{args: errutil.Args{"email": emails[i].Email}}
		}
	}

	if _, err := x.Insert(emails); err != nil {
		return fmt.Errorf("Insert: %v", err)
	}

	return nil
}

func (email *EmailAddress) Activate() error {
	user, err := Users.GetByID(context.TODO(), email.UserID)
	if err != nil {
		return err
	}
	if user.Rands, err = userutil.RandomSalt(); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	email.IsActivated = true
	if _, err := sess.ID(email.ID).AllCols().Update(email); err != nil {
		return err
	} else if err = updateUser(sess, user); err != nil {
		return err
	}

	return sess.Commit()
}

func DeleteEmailAddress(email *EmailAddress) (err error) {
	if email.ID > 0 {
		_, err = x.Id(email.ID).Delete(new(EmailAddress))
	} else {
		_, err = x.Where("email=?", email.Email).Delete(new(EmailAddress))
	}
	return err
}

func DeleteEmailAddresses(emails []*EmailAddress) (err error) {
	for i := range emails {
		if err = DeleteEmailAddress(emails[i]); err != nil {
			return err
		}
	}

	return nil
}

func MakeEmailPrimary(userID int64, email *EmailAddress) error {
	has, err := x.Get(email)
	if err != nil {
		return err
	} else if !has {
		return errors.EmailNotFound{Email: email.Email}
	}

	if email.UserID != userID {
		return errors.New("not the owner of the email")
	}

	if !email.IsActivated {
		return errors.EmailNotVerified{Email: email.Email}
	}

	user := &User{ID: email.UserID}
	has, err = x.Get(user)
	if err != nil {
		return err
	} else if !has {
		return ErrUserNotExist{args: map[string]interface{}{"userID": email.UserID}}
	}

	// Make sure the former primary email doesn't disappear.
	formerPrimaryEmail := &EmailAddress{Email: user.Email}
	has, err = x.Get(formerPrimaryEmail)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if !has {
		formerPrimaryEmail.UserID = user.ID
		formerPrimaryEmail.IsActivated = user.IsActive
		if _, err = sess.Insert(formerPrimaryEmail); err != nil {
			return err
		}
	}

	user.Email = email.Email
	if _, err = sess.ID(user.ID).AllCols().Update(user); err != nil {
		return err
	}

	return sess.Commit()
}
