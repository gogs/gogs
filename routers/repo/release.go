// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"sort"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

type ReleaseSorter struct {
	rels []*models.Release
}

func (rs *ReleaseSorter) Len() int {
	return len(rs.rels)
}

func (rs *ReleaseSorter) Less(i, j int) bool {
	return rs.rels[i].NumCommits > rs.rels[j].NumCommits
}

func (rs *ReleaseSorter) Swap(i, j int) {
	rs.rels[i], rs.rels[j] = rs.rels[j], rs.rels[i]
}

func Releases(ctx *middleware.Context) {
	ctx.Data["Title"] = "Releases"
	ctx.Data["IsRepoToolbarReleases"] = true
	ctx.Data["IsRepoReleaseNew"] = false
	rawTags, err := ctx.Repo.GitRepo.GetTags()
	if err != nil {
		ctx.Handle(500, "release.Releases(GetTags)", err)
		return
	}

	rels, err := models.GetReleasesByRepoId(ctx.Repo.Repository.Id)
	if err != nil {
		ctx.Handle(500, "release.Releases(GetReleasesByRepoId)", err)
		return
	}

	commitsCount, err := ctx.Repo.Commit.CommitsCount()
	if err != nil {
		ctx.Handle(500, "release.Releases(CommitsCount)", err)
		return
	}

	var tags ReleaseSorter
	tags.rels = make([]*models.Release, len(rawTags))
	for i, rawTag := range rawTags {
		for _, rel := range rels {
			if rel.TagName == rawTag {
				rel.Publisher, err = models.GetUserById(rel.PublisherId)
				if err != nil {
					ctx.Handle(500, "release.Releases(GetUserById)", err)
					return
				}
				rel.NumCommitsBehind = commitsCount - rel.NumCommits
				rel.Note = base.RenderMarkdownString(rel.Note, ctx.Repo.RepoLink)
				tags.rels[i] = rel
				break
			}
		}

		if tags.rels[i] == nil {
			commit, err := ctx.Repo.GitRepo.GetCommitOfTag(rawTag)
			if err != nil {
				ctx.Handle(500, "release.Releases(GetCommitOfTag)", err)
				return
			}

			tags.rels[i] = &models.Release{
				Title:   rawTag,
				TagName: rawTag,
				SHA1:    commit.Id.String(),
			}
			tags.rels[i].NumCommits, err = ctx.Repo.GitRepo.CommitsCount(commit.Id.String())
			if err != nil {
				ctx.Handle(500, "release.Releases(CommitsCount)", err)
				return
			}
			tags.rels[i].NumCommitsBehind = commitsCount - tags.rels[i].NumCommits
			tags.rels[i].Created = commit.Author.When
		}
	}

	sort.Sort(&tags)

	ctx.Data["Releases"] = tags.rels
	ctx.HTML(200, "release/list")
}

func ReleasesNew(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "release.ReleasesNew", nil)
		return
	}

	ctx.Data["Title"] = "New Release"
	ctx.Data["IsRepoToolbarReleases"] = true
	ctx.Data["IsRepoReleaseNew"] = true
	ctx.HTML(200, "release/new")
}

func ReleasesNewPost(ctx *middleware.Context, form auth.NewReleaseForm) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "release.ReleasesNew", nil)
		return
	}

	ctx.Data["Title"] = "New Release"
	ctx.Data["IsRepoToolbarReleases"] = true
	ctx.Data["IsRepoReleaseNew"] = true

	if ctx.HasError() {
		ctx.HTML(200, "release/new")
		return
	}

	commitsCount, err := ctx.Repo.Commit.CommitsCount()
	if err != nil {
		ctx.Handle(500, "release.ReleasesNewPost(CommitsCount)", err)
		return
	}

	rel := &models.Release{
		RepoId:       ctx.Repo.Repository.Id,
		PublisherId:  ctx.User.Id,
		Title:        form.Title,
		TagName:      form.TagName,
		SHA1:         ctx.Repo.Commit.Id.String(),
		NumCommits:   commitsCount,
		Note:         form.Content,
		IsPrerelease: form.Prerelease,
	}

	if err = models.CreateRelease(models.RepoPath(ctx.User.Name, ctx.Repo.Repository.Name),
		rel, ctx.Repo.GitRepo); err != nil {
		if err == models.ErrReleaseAlreadyExist {
			ctx.RenderWithErr("Release with this tag name has already existed", "release/new", &form)
		} else {
			ctx.Handle(500, "release.ReleasesNewPost(IsReleaseExist)", err)
		}
		return
	}
	log.Trace("%s Release created: %s/%s:%s", ctx.Req.RequestURI, ctx.User.LowerName, ctx.Repo.Repository.Name, form.TagName)

	ctx.Redirect(ctx.Repo.RepoLink + "/releases")
}
