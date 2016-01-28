// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers/api/v1/convert"
)

// Temporary: https://gist.github.com/sapk/df64347ff218baf4a277#get-a-branch
// https://github.com/gogits/go-gogs-client/wiki/Repositories-Branches#get-a-branch
func GetBranch(ctx *middleware.Context) {
	// Getting the branch requested
	branch, err := ctx.Repo.Repository.GetBranch(ctx.Params(":branchname"))
	if err != nil {
		ctx.APIError(500, "Repository.GetBranch", err)
		return
	}
	// Getting the last commit of the branch
	c, err := branch.GetCommit()
	if err != nil {
		ctx.APIError(500, "Branch.GetCommit", err)
		return
	}
	// Converting to API format and send payload
	ctx.JSON(200, convert.ToApiBranch(branch,c))
}

// Temporary: https://gist.github.com/sapk/df64347ff218baf4a277#list-branches
// https://github.com/gogits/go-gogs-client/wiki/Repositories-Branches#list-branches
func ListBranches(ctx *middleware.Context) {
	// Listing of branches
	Branches, err := ctx.Repo.Repository.GetBranches()
	if err != nil {
		ctx.APIError(500, "Repository.GetBranches", err)
		return
	}
	// Getting the last commit of each branch
	apiBranches := make([]*api.Branch, len(Branches))
	for i := range Branches {
		c, err := Branches[i].GetCommit()
		if err != nil {
			ctx.APIError(500, "Branch.GetCommit", err)
			return
		}
		// Converting to API format
		apiBranches[i] = convert.ToApiBranch(Branches[i],c)
	}
	// Sending the payload
	ctx.JSON(200, &apiBranches)
}
