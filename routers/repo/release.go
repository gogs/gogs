// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"

	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/form"
	"github.com/gogits/gogs/pkg/markup"
	"github.com/gogits/gogs/pkg/setting"
)

const (
	RELEASES    = "repo/release/list"
	RELEASE_NEW = "repo/release/new"
)

// calReleaseNumCommitsBehind calculates given release has how many commits behind release target.
func calReleaseNumCommitsBehind(repoCtx *context.Repository, release *models.Release, countCache map[string]int64) error {
	// Get count if not exists
	if _, ok := countCache[release.Target]; !ok {
		if repoCtx.GitRepo.IsBranchExist(release.Target) {
			commit, err := repoCtx.GitRepo.GetBranchCommit(release.Target)
			if err != nil {
				return fmt.Errorf("GetBranchCommit: %v", err)
			}
			countCache[release.Target], err = commit.CommitsCount()
			if err != nil {
				return fmt.Errorf("CommitsCount: %v", err)
			}
		} else {
			// Use NumCommits of the newest release on that target
			countCache[release.Target] = release.NumCommits
		}
	}
	release.NumCommitsBehind = countCache[release.Target] - release.NumCommits
	return nil
}

func Releases(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.releases")
	ctx.Data["PageIsViewFiles"] = true
	ctx.Data["PageIsReleaseList"] = true

	tagsResult, err := ctx.Repo.GitRepo.GetTagsAfter(ctx.Query("after"), 10)
	if err != nil {
		ctx.Handle(500, fmt.Sprintf("GetTags '%s'", ctx.Repo.Repository.RepoPath()), err)
		return
	}

	releases, err := models.GetPublishedReleasesByRepoID(ctx.Repo.Repository.ID, tagsResult.Tags...)
	if err != nil {
		ctx.Handle(500, "GetPublishedReleasesByRepoID", err)
		return
	}

	// Temproray cache commits count of used branches to speed up.
	countCache := make(map[string]int64)

	results := make([]*models.Release, len(tagsResult.Tags))
	for i, rawTag := range tagsResult.Tags {
		for j, r := range releases {
			if r == nil || r.TagName != rawTag {
				continue
			}
			releases[j] = nil // Mark as used.

			if err = r.LoadAttributes(); err != nil {
				ctx.Handle(500, "LoadAttributes", err)
				return
			}

			if err := calReleaseNumCommitsBehind(ctx.Repo, r, countCache); err != nil {
				ctx.Handle(500, "calReleaseNumCommitsBehind", err)
				return
			}

			r.Note = string(markup.Markdown(r.Note, ctx.Repo.RepoLink, ctx.Repo.Repository.ComposeMetas()))
			results[i] = r
			break
		}

		// No published release matches this tag
		if results[i] == nil {
			commit, err := ctx.Repo.GitRepo.GetTagCommit(rawTag)
			if err != nil {
				ctx.Handle(500, "GetTagCommit", err)
				return
			}

			results[i] = &models.Release{
				Title:   rawTag,
				TagName: rawTag,
				Sha1:    commit.ID.String(),
			}

			results[i].NumCommits, err = commit.CommitsCount()
			if err != nil {
				ctx.Handle(500, "CommitsCount", err)
				return
			}
			results[i].NumCommitsBehind = ctx.Repo.CommitsCount - results[i].NumCommits
		}
	}
	models.SortReleases(results)

	// Only show drafts if user is viewing the latest page
	var drafts []*models.Release
	if tagsResult.HasLatest {
		drafts, err = models.GetDraftReleasesByRepoID(ctx.Repo.Repository.ID)
		if err != nil {
			ctx.Handle(500, "GetDraftReleasesByRepoID", err)
			return
		}

		for _, r := range drafts {
			if err = r.LoadAttributes(); err != nil {
				ctx.Handle(500, "LoadAttributes", err)
				return
			}

			if err := calReleaseNumCommitsBehind(ctx.Repo, r, countCache); err != nil {
				ctx.Handle(500, "calReleaseNumCommitsBehind", err)
				return
			}

			r.Note = string(markup.Markdown(r.Note, ctx.Repo.RepoLink, ctx.Repo.Repository.ComposeMetas()))
		}

		if len(drafts) > 0 {
			results = append(drafts, results...)
		}
	}

	ctx.Data["Releases"] = results
	ctx.Data["HasPrevious"] = !tagsResult.HasLatest
	ctx.Data["ReachEnd"] = tagsResult.ReachEnd
	ctx.Data["PreviousAfter"] = tagsResult.PreviousAfter
	if len(results) > 0 {
		ctx.Data["NextAfter"] = results[len(results)-1].TagName
	}
	ctx.HTML(200, RELEASES)
}

func renderReleaseAttachmentSettings(ctx *context.Context) {
	ctx.Data["RequireDropzone"] = true
	ctx.Data["IsAttachmentEnabled"] = setting.Release.Attachment.Enabled
	ctx.Data["AttachmentAllowedTypes"] = strings.Join(setting.Release.Attachment.AllowedTypes, ",")
	ctx.Data["AttachmentMaxSize"] = setting.Release.Attachment.MaxSize
	ctx.Data["AttachmentMaxFiles"] = setting.Release.Attachment.MaxFiles
}

func NewRelease(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.new_release")
	ctx.Data["PageIsReleaseList"] = true
	ctx.Data["tag_target"] = ctx.Repo.Repository.DefaultBranch
	renderReleaseAttachmentSettings(ctx)
	ctx.HTML(200, RELEASE_NEW)
}

func NewReleasePost(ctx *context.Context, f form.NewRelease) {
	ctx.Data["Title"] = ctx.Tr("repo.release.new_release")
	ctx.Data["PageIsReleaseList"] = true
	renderReleaseAttachmentSettings(ctx)

	if ctx.HasError() {
		ctx.HTML(200, RELEASE_NEW)
		return
	}

	if !ctx.Repo.GitRepo.IsBranchExist(f.Target) {
		ctx.RenderWithErr(ctx.Tr("form.target_branch_not_exist"), RELEASE_NEW, &f)
		return
	}

	// Use current time if tag not yet exist, otherwise get time from Git
	var tagCreatedUnix int64
	tag, err := ctx.Repo.GitRepo.GetTag(f.TagName)
	if err == nil {
		commit, err := tag.Commit()
		if err == nil {
			tagCreatedUnix = commit.Author.When.Unix()
		}
	}

	commit, err := ctx.Repo.GitRepo.GetBranchCommit(f.Target)
	if err != nil {
		ctx.Handle(500, "GetBranchCommit", err)
		return
	}

	commitsCount, err := commit.CommitsCount()
	if err != nil {
		ctx.Handle(500, "CommitsCount", err)
		return
	}

	var attachments []string
	if setting.Release.Attachment.Enabled {
		attachments = f.Files
	}

	rel := &models.Release{
		RepoID:       ctx.Repo.Repository.ID,
		PublisherID:  ctx.User.ID,
		Title:        f.Title,
		TagName:      f.TagName,
		Target:       f.Target,
		Sha1:         commit.ID.String(),
		NumCommits:   commitsCount,
		Note:         f.Content,
		IsDraft:      len(f.Draft) > 0,
		IsPrerelease: f.Prerelease,
		CreatedUnix:  tagCreatedUnix,
	}
	if err = models.NewRelease(ctx.Repo.GitRepo, rel, attachments); err != nil {
		ctx.Data["Err_TagName"] = true
		switch {
		case models.IsErrReleaseAlreadyExist(err):
			ctx.RenderWithErr(ctx.Tr("repo.release.tag_name_already_exist"), RELEASE_NEW, &f)
		case models.IsErrInvalidTagName(err):
			ctx.RenderWithErr(ctx.Tr("repo.release.tag_name_invalid"), RELEASE_NEW, &f)
		default:
			ctx.Handle(500, "NewRelease", err)
		}
		return
	}
	log.Trace("Release created: %s/%s:%s", ctx.User.LowerName, ctx.Repo.Repository.Name, f.TagName)

	ctx.Redirect(ctx.Repo.RepoLink + "/releases")
}

func EditRelease(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.release.edit_release")
	ctx.Data["PageIsReleaseList"] = true
	ctx.Data["PageIsEditRelease"] = true
	renderReleaseAttachmentSettings(ctx)

	tagName := ctx.Params("*")
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
	ctx.Data["attachments"] = rel.Attachments
	ctx.Data["prerelease"] = rel.IsPrerelease
	ctx.Data["IsDraft"] = rel.IsDraft

	ctx.HTML(200, RELEASE_NEW)
}

func EditReleasePost(ctx *context.Context, f form.EditRelease) {
	ctx.Data["Title"] = ctx.Tr("repo.release.edit_release")
	ctx.Data["PageIsReleaseList"] = true
	ctx.Data["PageIsEditRelease"] = true
	renderReleaseAttachmentSettings(ctx)

	tagName := ctx.Params("*")
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
	ctx.Data["attachments"] = rel.Attachments
	ctx.Data["prerelease"] = rel.IsPrerelease
	ctx.Data["IsDraft"] = rel.IsDraft

	if ctx.HasError() {
		ctx.HTML(200, RELEASE_NEW)
		return
	}

	var attachments []string
	if setting.Release.Attachment.Enabled {
		attachments = f.Files
	}

	isPublish := rel.IsDraft && len(f.Draft) == 0
	rel.Title = f.Title
	rel.Note = f.Content
	rel.IsDraft = len(f.Draft) > 0
	rel.IsPrerelease = f.Prerelease
	if err = models.UpdateRelease(ctx.User, ctx.Repo.GitRepo, rel, isPublish, attachments); err != nil {
		ctx.Handle(500, "UpdateRelease", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/releases")
}

func UploadReleaseAttachment(ctx *context.Context) {
	if !setting.Release.Attachment.Enabled {
		ctx.NotFound()
		return
	}
	uploadAttachment(ctx, setting.Release.Attachment.AllowedTypes)
}

func DeleteRelease(ctx *context.Context) {
	if err := models.DeleteReleaseOfRepoByID(ctx.Repo.Repository.ID, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteReleaseByID: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.release.deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/releases",
	})
}
