// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"errors"

	api "github.com/gogits/go-gogs-client"

	base "github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/models"
)

func ToApiRelease(release *models.Release) *api.Release {

	return &api.Release{
		ID:                		release.Id,
		Publisher:            	*ToApiUser(release.Publisher),
		TagName:            	release.TagName,
		LowerTagName:        	release.LowerTagName,
		Target:                	release.Target,
		Title:                	release.Title,
		Sha1:                	release.Sha1,
		NumCommits:          	release.NumCommits,
		Note:               	release.Note,
		IsDraft:            	release.IsDraft,
		IsPrerelease:        	release.IsPrerelease,
		Created:            	release.Created,
	}
}

func ListReleases(ctx *middleware.Context) {
	rawTags, err := ctx.Repo.GitRepo.GetTags()
	if err != nil {
		ctx.Handle(500, "GetTags", err)
		return
	}

	rels, err := models.GetReleasesByRepoId(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "GetReleasesByRepoId", err)
		return
	}

	// Temproray cache commits count of used branches to speed up.
	countCache := make(map[string]int)

	tags := make([]*api.Release, len(rawTags))
	for i, rawTag := range rawTags {
		for j, rel := range rels {
			if rel == nil || (rel.IsDraft && !ctx.Repo.IsOwner()) {
				continue
			}
			if rel.TagName == rawTag {
				rel.Publisher, err = models.GetUserByID(rel.PublisherId)
				if err != nil {
					ctx.Handle(500, "GetUserById", err)
					return
				}
				// FIXME: duplicated code.
				// Get corresponding target if it's not the current branch.
				if ctx.Repo.BranchName != rel.Target {
					// Get count if not exists.
					if _, ok := countCache[rel.Target]; !ok {
						commit, err := ctx.Repo.GitRepo.GetCommitOfBranch(ctx.Repo.BranchName)
						if err != nil {
							ctx.Handle(500, "GetCommitOfBranch", err)
							return
						}
						countCache[ctx.Repo.BranchName], err = commit.CommitsCount()
						if err != nil {
							ctx.Handle(500, "CommitsCount2", err)
							return
						}
					}
					rel.NumCommitsBehind = countCache[ctx.Repo.BranchName] - rel.NumCommits
				} else {
					rel.NumCommitsBehind = ctx.Repo.CommitsCount - rel.NumCommits
				}

				rel.Note = base.RenderMarkdownString(rel.Note, ctx.Repo.RepoLink)
				tags[i] = ToApiRelease(rel)
				rels[j] = nil // Mark as used.
				break
			}
		}

		if tags[i] == nil {
			commit, err := ctx.Repo.GitRepo.GetCommitOfTag(rawTag)
			if err != nil {
				ctx.Handle(500, "GetCommitOfTag2", err)
				return
			}

			tags[i] = &api.Release{
				Title:   rawTag,
				TagName: rawTag,
				Sha1:    commit.Id.String(),
			}

			tags[i].NumCommits, err = ctx.Repo.GitRepo.CommitsCount(commit.Id.String())
			if err != nil {
				ctx.Handle(500, "CommitsCount", err)
				return
			}
			tags[i].NumCommitsBehind = ctx.Repo.CommitsCount - tags[i].NumCommits
		}
	}

	for _, rel := range rels {
		if rel == nil {
			continue
		}

		rel.Publisher, err = models.GetUserByID(rel.PublisherId)
		if err != nil {
			ctx.Handle(500, "GetUserById", err)
			return
		}
		// FIXME: duplicated code.
		// Get corresponding target if it's not the current branch.
		if ctx.Repo.BranchName != rel.Target {
			// Get count if not exists.
			if _, ok := countCache[rel.Target]; !ok {
				commit, err := ctx.Repo.GitRepo.GetCommitOfBranch(ctx.Repo.BranchName)
				if err != nil {
					ctx.Handle(500, "GetCommitOfBranch", err)
					return
				}
				countCache[ctx.Repo.BranchName], err = commit.CommitsCount()
				if err != nil {
					ctx.Handle(500, "CommitsCount2", err)
					return
				}
			}
			rel.NumCommitsBehind = countCache[ctx.Repo.BranchName] - rel.NumCommits
		} else {
			rel.NumCommitsBehind = ctx.Repo.CommitsCount - rel.NumCommits
		}

		rel.Note = base.RenderMarkdownString(rel.Note, ctx.Repo.RepoLink)
		tags = append(tags, ToApiRelease(rel))
	}

	ctx.JSON(200, &tags)
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
		if models.IsErrUserNotExist(err) {
			publisher = models.NewFakeUser()
		} else {
			ctx.Handle(422, "GetUserByID", err)
			return
		}
	}
	rel.Publisher = publisher

	ctx.JSON(200, ToApiRelease(rel))
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
		Publisher:    ctx.User,
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
		ctx.Handle(500, "CreateRelease", err)
		return
	}


	ctx.JSON(201, ToApiRelease(rel))
}