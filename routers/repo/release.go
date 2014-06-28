// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	RELEASES     base.TplName = "repo/release/list"
	RELEASE_NEW  base.TplName = "repo/release/new"
	RELEASE_EDIT base.TplName = "repo/release/edit"
)

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

	// Temproray cache commits count of used branches to speed up.
	countCache := make(map[string]int)

	tags := make([]*models.Release, len(rawTags))
	for i, rawTag := range rawTags {
		for _, rel := range rels {
			if rel.IsDraft && !ctx.Repo.IsOwner {
				continue
			}
			if rel.TagName == rawTag {
				rel.Publisher, err = models.GetUserById(rel.PublisherId)
				if err != nil {
					ctx.Handle(500, "release.Releases(GetUserById)", err)
					return
				}
				// Get corresponding target if it's not the current branch.
				if ctx.Repo.BranchName != rel.Target {
					// Get count if not exists.
					if _, ok := countCache[rel.Target]; !ok {
						commit, err := ctx.Repo.GitRepo.GetCommitOfTag(rel.TagName)
						if err != nil {
							ctx.Handle(500, "release.Releases(GetCommitOfTag)", err)
							return
						}
						countCache[rel.Target], err = commit.CommitsCount()
						if err != nil {
							ctx.Handle(500, "release.Releases(CommitsCount2)", err)
							return
						}
					}
					rel.NumCommitsBehind = countCache[rel.Target] - rel.NumCommits
				} else {
					rel.NumCommitsBehind = commitsCount - rel.NumCommits
				}

				rel.Note = base.RenderMarkdownString(rel.Note, ctx.Repo.RepoLink)
				tags[i] = rel
				break
			}
		}

		if tags[i] == nil {
			commit, err := ctx.Repo.GitRepo.GetCommitOfTag(rawTag)
			if err != nil {
				ctx.Handle(500, "release.Releases(GetCommitOfTag2)", err)
				return
			}

			tags[i] = &models.Release{
				Title:   rawTag,
				TagName: rawTag,
				Sha1:    commit.Id.String(),
			}

			tags[i].NumCommits, err = ctx.Repo.GitRepo.CommitsCount(commit.Id.String())
			if err != nil {
				ctx.Handle(500, "release.Releases(CommitsCount)", err)
				return
			}
			tags[i].NumCommitsBehind = commitsCount - tags[i].NumCommits
		}
	}
	models.SortReleases(tags)
	ctx.Data["Releases"] = tags
	ctx.HTML(200, RELEASES)
}

func NewRelease(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(403, "release.ReleasesNew", nil)
		return
	}

	ctx.Data["Title"] = "New Release"
	ctx.Data["IsRepoToolbarReleases"] = true
	ctx.Data["IsRepoReleaseNew"] = true
	ctx.HTML(200, RELEASE_NEW)
}

func NewReleasePost(ctx *middleware.Context, form auth.NewReleaseForm) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(403, "release.ReleasesNew", nil)
		return
	}

	ctx.Data["Title"] = "New Release"
	ctx.Data["IsRepoToolbarReleases"] = true
	ctx.Data["IsRepoReleaseNew"] = true

	if ctx.HasError() {
		ctx.HTML(200, RELEASE_NEW)
		return
	}

	commitsCount, err := ctx.Repo.Commit.CommitsCount()
	if err != nil {
		ctx.Handle(500, "release.ReleasesNewPost(CommitsCount)", err)
		return
	}

	if !ctx.Repo.GitRepo.IsBranchExist(form.Target) {
		ctx.RenderWithErr("Target branch does not exist", "release/new", &form)
		return
	}

	rel := &models.Release{
		RepoId:       ctx.Repo.Repository.Id,
		PublisherId:  ctx.User.Id,
		Title:        form.Title,
		TagName:      form.TagName,
		Target:       form.Target,
		Sha1:         ctx.Repo.Commit.Id.String(),
		NumCommits:   commitsCount,
		Note:         form.Content,
		IsDraft:      len(form.Draft) > 0,
		IsPrerelease: form.Prerelease,
	}

	if err = models.CreateRelease(ctx.Repo.GitRepo, rel); err != nil {
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

func EditRelease(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(403, "release.ReleasesEdit", nil)
		return
	}

	tagName := params["tagname"]
	rel, err := models.GetRelease(ctx.Repo.Repository.Id, tagName)
	if err != nil {
		if err == models.ErrReleaseNotExist {
			ctx.Handle(404, "release.ReleasesEdit(GetRelease)", err)
		} else {
			ctx.Handle(500, "release.ReleasesEdit(GetRelease)", err)
		}
		return
	}
	ctx.Data["Release"] = rel

	ctx.Data["Title"] = "Edit Release"
	ctx.Data["IsRepoToolbarReleases"] = true
	ctx.HTML(200, RELEASE_EDIT)
}

func EditReleasePost(ctx *middleware.Context, params martini.Params, form auth.EditReleaseForm) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(403, "release.EditReleasePost", nil)
		return
	}

	tagName := params["tagname"]
	rel, err := models.GetRelease(ctx.Repo.Repository.Id, tagName)
	if err != nil {
		if err == models.ErrReleaseNotExist {
			ctx.Handle(404, "release.EditReleasePost(GetRelease)", err)
		} else {
			ctx.Handle(500, "release.EditReleasePost(GetRelease)", err)
		}
		return
	}
	ctx.Data["Release"] = rel

	if ctx.HasError() {
		ctx.HTML(200, RELEASE_EDIT)
		return
	}

	ctx.Data["Title"] = "Edit Release"
	ctx.Data["IsRepoToolbarReleases"] = true

	rel.Title = form.Title
	rel.Note = form.Content
	rel.IsDraft = len(form.Draft) > 0
	rel.IsPrerelease = form.Prerelease
	if err = models.UpdateRelease(ctx.Repo.GitRepo, rel); err != nil {
		ctx.Handle(500, "release.EditReleasePost(UpdateRelease)", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/releases")
}
