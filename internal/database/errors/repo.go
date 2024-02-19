// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import (
	"fmt"
)

type InvalidRepoReference struct {
	Ref string
}

func IsInvalidRepoReference(err error) bool {
	_, ok := err.(InvalidRepoReference)
	return ok
}

func (err InvalidRepoReference) Error() string {
	return fmt.Sprintf("invalid repository reference [ref: %s]", err.Ref)
}

type MirrorNotExist struct {
	RepoID int64
}

func IsMirrorNotExist(err error) bool {
	_, ok := err.(MirrorNotExist)
	return ok
}

func (err MirrorNotExist) Error() string {
	return fmt.Sprintf("mirror does not exist [repo_id: %d]", err.RepoID)
}

type BranchAlreadyExists struct {
	Name string
}

func IsBranchAlreadyExists(err error) bool {
	_, ok := err.(BranchAlreadyExists)
	return ok
}

func (err BranchAlreadyExists) Error() string {
	return fmt.Sprintf("branch already exists [name: %s]", err.Name)
}
