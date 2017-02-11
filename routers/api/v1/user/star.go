// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/models"
	"fmt"
	"github.com/gogits/gogs/routers/api/v1/repo"
)

func getStarredRepos(userID int64) ([]*api.Repository, error) {
	starred_repos, err := models.GetStarredRepos(userID)
	if err != nil {
		return nil, err
	}
	repos := make([]*api.Repository, len(starred_repos))
	for i, starred := range starred_repos {
		repos[i] = starred.APIFormat(&api.Permission{true, true, true})
	}
	return repos, nil
}

func GetStarredRepos(ctx *context.APIContext) {
	user := GetUserByParams(ctx)
	repos, err := getStarredRepos(user.ID)
	if err != nil {
		ctx.Error(500, "getStarredRepos", err)
	}
	ctx.JSON(200, &repos)
}

func GetMyStarredRepos(ctx *context.APIContext) {
	repos, err := getStarredRepos(ctx.User.ID)
	if err != nil {
		ctx.Error(500, "getStarredRepos", err)
	}
	ctx.JSON(200, &repos)
}

func IsStarring(ctx *context.APIContext) {
	fmt.Print("IsStarring called\n")
	_, repository := repo.ParseOwnerAndRepo(ctx)
	if ctx.Written() {
		return;
	}
	repoID := repository.ID;
	starred, err := models.GetStarredRepos(ctx.User.ID);
	if err != nil {
		ctx.Error(500, "IsStarring", err)
	}
	for _, repository := range starred {
		if repository.ID == repoID {
			ctx.Status(204);
		}
	}
	ctx.Status(404);
}

func Star(ctx *context.APIContext) {
    _, repository := repo.ParseOwnerAndRepo(ctx);
	if ctx.Written() {
		return
	}
	userID := ctx.User.ID
	repoID := repository.ID
	models.StarRepo(userID, repoID, true)
	ctx.Status(204)
}

func Unstar(ctx *context.APIContext) {
	_, repository := repo.ParseOwnerAndRepo(ctx)
	if ctx.Written() {
		return
	}
	userID := ctx.User.ID
	repoID := repository.ID
	models.StarRepo(userID, repoID, false)
	ctx.Status(204)
}
