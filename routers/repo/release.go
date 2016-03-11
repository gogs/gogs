// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/markdown"
)

const (
	RELEASES    base.TplName = "repo/release/list"
	RELEASE_NEW base.TplName = "repo/release/new"
)

// calReleaseNumCommitsBehind calculates given release has how many commits behind default branch.
func calReleaseNumCommitsBehind(repoCtx *context.Repository, release *models.Release, countCache map[string]int64) error {
	// Fast return if release target is same as default branch.
	if repoCtx.BranchName == release.Target {
		release.NumCommitsBehind = repoCtx.CommitsCount - release.NumCommits
		return nil
	}

	// Get count if not exists
	if _, ok := countCache[release.Target]; !ok {
		commit, err := repoCtx.GitRepo.GetBranchCommit(repoCtx.BranchName)
		if err != nil {
			return fmt.Errorf("GetBranchCommit: %v", err)
		}
		countCache[repoCtx.BranchName], err = commit.CommitsCount()
		if err != nil {
			return fmt.Errorf("CommitsCount: %v", err)
		}
	}
	release.NumCommitsBehind = countCache[repoCtx.BranchName] - release.NumCommits
	return nil
}

func Releases(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.releases")
	ctx.Data["PageIsReleaseList"] = true

	rawTags, err := ctx.Repo.GitRepo.GetTags()
	if err != nil {
		ctx.Handle(500, "GetTags", err)
		return
	}

	releases, err := models.GetReleasesByRepoID(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "GetReleasesByRepoID", err)
		return
	}

	// Temproray cache commits count of used branches to speed up.
	countCache := make(map[string]int64)

	tags := make([]*models.Release, len(rawTags))
	for i, rawTag := range rawTags {
		for j, r := range releases {
			if r == nil || (r.IsDraft && !ctx.Repo.IsOwner()) {
				continue
			}
			if r.TagName == rawTag {
				r.Publisher, err = models.GetUserByID(r.PublisherID)
				if err != nil {
					if models.IsErrUserNotExist(err) {
						r.Publisher = models.NewFakeUser()
					} else {
						ctx.Handle(500, "GetUserByID", err)
						return
					}
				}

				if err := calReleaseNumCommitsBehind(ctx.Repo, r, countCache); err != nil {
					ctx.Handle(500, "calReleaseNumCommitsBehind", err)
					return
				}

				r.Note = markdown.RenderString(r.Note, ctx.Repo.RepoLink, ctx.Repo.Repository.ComposeMetas())
				tags[i] = r
				releases[j] = nil // Mark as used.
				break
			}
		}

		if tags[i] == nil {
			commit, err := ctx.Repo.GitRepo.GetTagCommit(rawTag)
			if err != nil {
				ctx.Handle(500, "GetTagCommit", err)
				return
			}

			tags[i] = &models.Release{
				Title:   rawTag,
				TagName: rawTag,
				Sha1:    commit.ID.String(),
			}

			tags[i].NumCommits, err = commit.CommitsCount()
			if err != nil {
				ctx.Handle(500, "CommitsCount", err)
				return
			}
			tags[i].NumCommitsBehind = ctx.Repo.CommitsCount - tags[i].NumCommits
		}
	}

	for _, r := range releases {
		if r == nil {
			continue
		}

		r.Publisher, err = models.GetUserByID(r.PublisherID)
		if err != nil {
			if models.IsErrUserNotExist(err) {
				r.Publisher = models.NewFakeUser()
			} else {
				ctx.Handle(500, "GetUserByID", err)
				return
			}
		}

		if err := calReleaseNumCommitsBehind(ctx.Repo, r, countCache); err != nil {
			ctx.Handle(500, "calReleaseNumCommitsBehind", err)
			return
		}

		r.Note = markdown.RenderString(r.Note, ctx.Repo.RepoLink, ctx.Repo.Repository.ComposeMetas())
		tags = append(tags, r)
	}
	models.SortReleases(tags)
	ctx.Data["Releases"] = tags
	ctx.HTML(200, RELEASES)
}

func NewRelease(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.new_release")
	ctx.Data["PageIsReleaseList"] = true
	ctx.Data["tag_target"] = ctx.Repo.Repository.DefaultBranch
	ctx.HTML(200, RELEASE_NEW)
}

func NewReleasePost(ctx *context.Context, form auth.NewReleaseForm) {
	ctx.Data["Title"] = ctx.Tr("repo.release.new_release")
	ctx.Data["PageIsReleaseList"] = true

	if ctx.HasError() {
		ctx.HTML(200, RELEASE_NEW)
		return
	}

	if !ctx.Repo.GitRepo.IsBranchExist(form.Target) {
		ctx.RenderWithErr(ctx.Tr("form.target_branch_not_exist"), RELEASE_NEW, &form)
		return
	}

	commit, err := ctx.Repo.GitRepo.GetBranchCommit(form.Target)
	if err != nil {
		ctx.Handle(500, "GetBranchCommit", err)
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

func EditRelease(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.edit_release")
	ctx.Data["PageIsReleaseList"] = true
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
	ctx.Data["ID"] = rel.ID
	ctx.Data["tag_name"] = rel.TagName
	ctx.Data["tag_target"] = rel.Target
	ctx.Data["title"] = rel.Title
	ctx.Data["content"] = rel.Note
	ctx.Data["prerelease"] = rel.IsPrerelease

	ctx.HTML(200, RELEASE_NEW)
}

func EditReleasePost(ctx *context.Context, form auth.EditReleaseForm) {
	ctx.Data["Title"] = ctx.Tr("repo.release.edit_release")
	ctx.Data["PageIsReleaseList"] = true
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

func DeleteRelease(ctx *context.Context) {
	if err := models.DeleteReleaseByID(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteReleaseByID: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.release.deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/releases",
	})
}
