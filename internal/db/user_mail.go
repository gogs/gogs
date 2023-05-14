// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"

	"gogs.io/gogs/internal/db/errors"
)

func (email *EmailAddress) Activate() error {
	email.IsActivated = true
	if _, err := x.ID(email.ID).AllCols().Update(email); err != nil {
		return err
	}
	return Users.Update(context.TODO(), email.UserID, UpdateUserOptions{GenerateNewRands: true})
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
		return ErrUserNotExist{args: map[string]any{"userID": email.UserID}}
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
