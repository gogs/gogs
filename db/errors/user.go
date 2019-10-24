// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

type EmptyName struct{}

func IsEmptyName(err error) bool {
	_, ok := err.(EmptyName)
	return ok
}

func (err EmptyName) Error() string {
	return "empty name"
}

type UserNotExist struct {
	UserID int64
	Name   string
}

func IsUserNotExist(err error) bool {
	_, ok := err.(UserNotExist)
	return ok
}

func (err UserNotExist) Error() string {
	return fmt.Sprintf("user does not exist [user_id: %d, name: %s]", err.UserID, err.Name)
}

type UserNotKeyOwner struct {
	KeyID int64
}

func IsUserNotKeyOwner(err error) bool {
	_, ok := err.(UserNotKeyOwner)
	return ok
}

func (err UserNotKeyOwner) Error() string {
	return fmt.Sprintf("user is not the owner of public key [key_id: %d]", err.KeyID)
}
