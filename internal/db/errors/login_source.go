// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

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
