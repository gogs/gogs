// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

type IssueNotExist struct {
	ID     int64
	RepoID int64
	Index  int64
}

func IsIssueNotExist(err error) bool {
	_, ok := err.(IssueNotExist)
	return ok
}

func (err IssueNotExist) Error() string {
	return fmt.Sprintf("issue does not exist [id: %d, repo_id: %d, index: %d]", err.ID, err.RepoID, err.Index)
}

type InvalidIssueReference struct {
	Ref string
}

func IsInvalidIssueReference(err error) bool {
	_, ok := err.(InvalidIssueReference)
	return ok
}

func (err InvalidIssueReference) Error() string {
	return fmt.Sprintf("invalid issue reference [ref: %s]", err.Ref)
}
