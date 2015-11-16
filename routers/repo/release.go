// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	RELEASES    base.TplName = "repo/release/list"
	RELEASE_NEW base.TplName = "repo/release/new"
)

func Releases(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.releases")

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

	tags := make([]*models.Release, len(rawTags))
	for i, rawTag := range rawTags {
		for j, rel := range rels {
			if rel == nil || (rel.IsDraft && !ctx.Repo.IsOwner()) {
				continue
			}
			if rel.TagName == rawTag {
				rel.Publisher, err = models.GetUserByID(rel.PublisherID)
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
				tags[i] = rel
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

			tags[i] = &models.Release{
				Title:   rawTag,
				TagName: rawTag,
				Sha1:    commit.ID.String(),
			}

			tags[i].NumCommits, err = ctx.Repo.GitRepo.CommitsCount(commit.ID.String())
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

		rel.Publisher, err = models.GetUserByID(rel.PublisherID)
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
		tags = append(tags, rel)
	}
	models.SortReleases(tags)
	ctx.Data["Releases"] = tags
	ctx.HTML(200, RELEASES)
}

func NewRelease(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.new_release")
	ctx.Data["tag_target"] = ctx.Repo.Repository.DefaultBranch
	ctx.HTML(200, RELEASE_NEW)
}

func NewReleasePost(ctx *middleware.Context, form auth.NewReleaseForm) {
	ctx.Data["Title"] = ctx.Tr("repo.release.new_release")

	if ctx.HasError() {
		ctx.HTML(200, RELEASE_NEW)
		return
	}

	if !ctx.Repo.GitRepo.IsBranchExist(form.Target) {
		ctx.RenderWithErr(ctx.Tr("form.target_branch_not_exist"), RELEASE_NEW, &form)
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
		RepoID:       ctx.Repo.Repository.ID,
		PublisherID:  ctx.User.Id,
		Title:        form.Title,
		TagName:      form.TagName,
		Target:       form.Target,
		Sha1:         commit.ID.String(),
		NumCommits:   commitsCount,
		Note:         form.Content,
		IsDraft:      len(form.Draft) > 0,
		IsPrerelease: form.Prerelease,
	}

	if err = models.CreateRelease(ctx.Repo.GitRepo, rel); err != nil {
		if models.IsErrReleaseAlreadyExist(err) {
			ctx.Data["Err_TagName"] = true
			ctx.RenderWithErr(ctx.Tr("repo.release.tag_name_already_exist"), RELEASE_NEW, &form)
		} else {
			ctx.Handle(500, "CreateRelease", err)
		}
		return
	}
	log.Trace("Release created: %s/%s:%s", ctx.User.LowerName, ctx.Repo.Repository.Name, form.TagName)

	ctx.Redirect(ctx.Repo.RepoLink + "/releases")
}

func EditRelease(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.edit_release")
	ctx.Data["PageIsEditRelease"] = true

	tagName := ctx.Params(":tagname")
	rel, err := models.GetRelease(ctx.Repo.Repository.ID, tagName)
	if err != nil {
		if models.IsErrReleaseNotExist(err) {
			ctx.Handle(404, "GetRelease", err)
		} else {
			ctx.Handle(500, "GetRelease", err)
		}
		return
	}
	ctx.Data["tag_name"] = rel.TagName
	ctx.Data["tag_target"] = rel.Target
	ctx.Data["title"] = rel.Title
	ctx.Data["content"] = rel.Note
	ctx.Data["prerelease"] = rel.IsPrerelease

	ctx.HTML(200, RELEASE_NEW)
}

func EditReleasePost(ctx *middleware.Context, form auth.EditReleaseForm) {
	ctx.Data["Title"] = ctx.Tr("repo.release.edit_release")
	ctx.Data["PageIsEditRelease"] = true

	tagName := ctx.Params(":tagname")
	rel, err := models.GetRelease(ctx.Repo.Repository.ID, tagName)
	if err != nil {
		if models.IsErrReleaseNotExist(err) {
			ctx.Handle(404, "GetRelease", err)
		} else {
			ctx.Handle(500, "GetRelease", err)
		}
		return
	}
	ctx.Data["tag_name"] = rel.TagName
	ctx.Data["tag_target"] = rel.Target
	ctx.Data["title"] = rel.Title
	ctx.Data["content"] = rel.Note
	ctx.Data["prerelease"] = rel.IsPrerelease

	if ctx.HasError() {
		ctx.HTML(200, RELEASE_NEW)
		return
	}

	rel.Title = form.Title
	rel.Note = form.Content
	rel.IsDraft = len(form.Draft) > 0
	rel.IsPrerelease = form.Prerelease
	if err = models.UpdateRelease(ctx.Repo.GitRepo, rel); err != nil {
		ctx.Handle(500, "UpdateRelease", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/releases")
}
