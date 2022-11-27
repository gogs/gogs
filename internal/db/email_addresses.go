// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errutil"
)

// EmailAddressesStore is the persistent interface for email addresses.
//
// NOTE: All methods are sorted in alphabetical order.
type EmailAddressesStore interface {
	// GetByEmail returns the email address with given email. If `needsActivated` is
	// true, only activated email will be returned, otherwise, it may return
	// inactivated email addresses. It returns ErrEmailNotExist when no qualified
	// email is not found.
	GetByEmail(ctx context.Context, email string, needsActivated bool) (*EmailAddress, error)
}

var EmailAddresses EmailAddressesStore

var _ EmailAddressesStore = (*emailAddresses)(nil)

type emailAddresses struct {
	*gorm.DB
}

// NewEmailAddressesStore returns a persistent interface for email addresses
// with given database connection.
func NewEmailAddressesStore(db *gorm.DB) EmailAddressesStore {
	return &emailAddresses{DB: db}
}

var _ errutil.NotFound = (*ErrEmailNotExist)(nil)

type ErrEmailNotExist struct {
	args errutil.Args
}

func IsErrEmailAddressNotExist(err error) bool {
	_, ok := errors.Cause(err).(ErrEmailNotExist)
	return ok
}

func (err ErrEmailNotExist) Error() string {
	return fmt.Sprintf("email address does not exist: %v", err.args)
}

func (ErrEmailNotExist) NotFound() bool {
	return true
}

func (db *emailAddresses) GetByEmail(ctx context.Context, email string, needsActivated bool) (*EmailAddress, error) {
	tx := db.WithContext(ctx).Where("email = ?", email)
	if needsActivated {
		tx = tx.Where("is_activated = ?", true)
	}

	emailAddress := new(EmailAddress)
	err := tx.First(emailAddress).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrEmailNotExist{
				args: errutil.Args{
					"email": email,
				},
			}
		}
		return nil, err
	}
	return emailAddress, nil
}
