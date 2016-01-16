// Copyright 2014 The Gogs Authors. All rights reserved.
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
	branch, err := ctx.Repo.Repository.GetBranch(ctx.Params(":id"))
	if err != nil {
		//TODO handle error
		return
	}
	c, err := branch.GetCommit()
	if err != nil {
		//TODO handle error
		return
	}
	ctx.JSON(200, convert.ToApiBranch(branch,c))
}

// Temporary: https://gist.github.com/sapk/df64347ff218baf4a277#list-branches
// https://github.com/gogits/go-gogs-client/wiki/Repositories-Branches#list-branches
func ListBranches(ctx *middleware.Context) {
	Branches, err := ctx.Repo.Repository.GetBranches()
	if err != nil {
		//TODO handle error
		return
	}
	apiBranches := make([]*api.Branch, len(Branches))
	for i := range Branches {
		c, err := Branches[i].GetCommit()
		if err != nil {
			//TODO handle error
			continue
		}
		apiBranches[i] = convert.ToApiBranch(Branches[i],c)
	}

	ctx.JSON(200, &apiBranches)
}
