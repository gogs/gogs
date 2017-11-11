// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

type EmailNotFound struct {
	Email string
}

func IsEmailNotFound(err error) bool {
	_, ok := err.(EmailNotFound)
	return ok
}

func (err EmailNotFound) Error() string {
	return fmt.Sprintf("email is not found [email: %s]", err.Email)
}

type EmailNotVerified struct {
	Email string
}

func IsEmailNotVerified(err error) bool {
	_, ok := err.(EmailNotVerified)
	return ok
}

func (err EmailNotVerified) Error() string {
	return fmt.Sprintf("email has not been verified [email: %s]", err.Email)
}
