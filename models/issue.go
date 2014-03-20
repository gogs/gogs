// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

type Issue struct {
	Id       int64
	RepoId   int64 `xorm:"index"`
	PosterId int64
}

type PullRequest struct {
	Id int64
}

type Comment struct {
	Id int64
}
