// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"errors"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/models"
)

func ToApiRelease(release *models.Release, publisher *models.User) *api.Release {

	return &api.Release{
		Id:          		release.Id,
		Publisher:			*ToApiUser(publisher),
		TagName:			release.TagName,
		LowerTagName:		release.LowerTagName,
		Target:				release.Target,
		Title:				release.Title,
		Sha1:				release.Sha1,
		NumCommits:			release.NumCommits,
		Note:				release.Note,
		IsDraft:			release.IsDraft,
		IsPrerelease:		release.IsPrerelease,
		Created:			release.Created,
	}
}

func ListReleases(ctx *middleware.Context) {
	rels, err := models.GetReleasesByRepoId(ctx.Repo.Repository.ID)
	if err != nil {
		log.Error(4, "GetReleasesByRepoId: %v", err)
		ctx.Status(500)
		return
	}

	apiReleases := make([]*api.Release, len(rels))
	for i, rel := range rels {
		publisher, err := models.GetUserByID(rel.PublisherId)
		if err != nil {
			log.Error(4, "GetUserByID: %v", err)
			return
		}

		apiReleases[i] = ToApiRelease(rel, publisher)
	}
	ctx.JSON(200, &apiReleases)
}

func ReleaseByName(ctx *middleware.Context) {
	rel, err := models.GetRelease(ctx.Repo.Repository.ID, ctx.Params(":release"))
	if err != nil {
		log.Error(4, "GetRelease: %v", err)
		ctx.Status(500)
		return
	}

	publisher, err := models.GetUserByID(rel.PublisherId)
	if err != nil {
		log.Error(4, "GetUserByID: %v", err)
		return
	}

	ctx.JSON(200, ToApiRelease(rel, publisher))
}


func CreateRelease(ctx *middleware.Context, form api.CreateReleaseOption) {
	if !ctx.Repo.GitRepo.IsBranchExist(form.Target) {
		ctx.Handle(400, "IsBranchExist", errors.New("Branch did not exist, " + form.Target))
		return
	}

	commit, err := ctx.Repo.GitRepo.GetCommitOfBranch(form.Target)
	if err != nil {
		ctx.Handle(500, "GetCommitOfBranch", err)
		return
	}

	commitsCount, err := commit.CommitsCount()
	if err != nil {
		ctx.Handle(500, "CommitsCount", err)
		return
	}

	rel := &models.Release{
		RepoId:       ctx.Repo.Repository.ID,
		PublisherId:  ctx.User.Id,
		Title:        form.Title,
		TagName:      form.TagName,
		Target:       form.Target,
		Sha1:         commit.Id.String(),
		NumCommits:   commitsCount,
		Note:         form.Note,
		IsDraft:      form.IsDraft,
		IsPrerelease: form.IsPrerelease,
	}

	err = models.CreateRelease(ctx.Repo.GitRepo, rel)
	if err != nil {
		ctx.Handle(400, "CreateRelease", err)
		return
	}


	ctx.JSON(201, ToApiRelease(rel, ctx.User))
}