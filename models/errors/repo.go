// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

type RepoNotExist struct {
	ID     int64
	UserID int64
	Name   string
}

func IsRepoNotExist(err error) bool {
	_, ok := err.(RepoNotExist)
	return ok
}

func (err RepoNotExist) Error() string {
	return fmt.Sprintf("repository does not exist [id: %d, user_id: %d, name: %s]", err.ID, err.UserID, err.Name)
}

type ReachLimitOfRepo struct {
	Limit int
}

func IsReachLimitOfRepo(err error) bool {
	_, ok := err.(ReachLimitOfRepo)
	return ok
}

func (err ReachLimitOfRepo) Error() string {
	return fmt.Sprintf("user has reached maximum limit of repositories [limit: %d]", err.Limit)
}

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
