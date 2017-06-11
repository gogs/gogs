// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/routes/api/v1/convert"
)

// https://github.com/gogits/go-gogs-client/wiki/Repositories#get-branch
func GetBranch(c *context.APIContext) {
	branch, err := c.Repo.Repository.GetBranch(c.Params("*"))
	if err != nil {
		if models.IsErrBranchNotExist(err) {
			c.Error(404, "GetBranch", err)
		} else {
			c.Error(500, "GetBranch", err)
		}
		return
	}

	commit, err := branch.GetCommit()
	if err != nil {
		c.Error(500, "GetCommit", err)
		return
	}

	c.JSON(200, convert.ToBranch(branch, commit))
}

// https://github.com/gogits/go-gogs-client/wiki/Repositories#list-branches
func ListBranches(c *context.APIContext) {
	branches, err := c.Repo.Repository.GetBranches()
	if err != nil {
		c.Error(500, "GetBranches", err)
		return
	}

	apiBranches := make([]*api.Branch, len(branches))
	for i := range branches {
		commit, err := branches[i].GetCommit()
		if err != nil {
			c.Error(500, "GetCommit", err)
			return
		}
		apiBranches[i] = convert.ToBranch(branches[i], commit)
	}

	c.JSON(200, &apiBranches)
}
