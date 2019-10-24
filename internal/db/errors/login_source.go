// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

type LoginSourceNotExist struct {
	ID int64
}

func IsLoginSourceNotExist(err error) bool {
	_, ok := err.(LoginSourceNotExist)
	return ok
}

func (err LoginSourceNotExist) Error() string {
	return fmt.Sprintf("login source does not exist [id: %d]", err.ID)
}

type LoginSourceNotActivated struct {
	SourceID int64
}

func IsLoginSourceNotActivated(err error) bool {
	_, ok := err.(LoginSourceNotActivated)
	return ok
}

func (err LoginSourceNotActivated) Error() string {
	return fmt.Sprintf("login source is not activated [source_id: %d]", err.SourceID)
}

type InvalidLoginSourceType struct {
	Type interface{}
}

func IsInvalidLoginSourceType(err error) bool {
	_, ok := err.(InvalidLoginSourceType)
	return ok
}

func (err InvalidLoginSourceType) Error() string {
	return fmt.Sprintf("invalid login source type [type: %v]", err.Type)
}

type LoginSourceMismatch struct {
	Expect int64
	Actual int64
}

func IsLoginSourceMismatch(err error) bool {
	_, ok := err.(LoginSourceMismatch)
	return ok
}

func (err LoginSourceMismatch) Error() string {
	return fmt.Sprintf("login source mismatch [expect: %d, actual: %d]", err.Expect, err.Actual)
}
