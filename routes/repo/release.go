// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"

	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/markup"
	"github.com/gogs/gogs/pkg/setting"
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

func Releases(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.release.releases")
	c.Data["PageIsViewFiles"] = true
	c.Data["PageIsReleaseList"] = true

	tagsResult, err := c.Repo.GitRepo.GetTagsAfter(c.Query("after"), 10)
	if err != nil {
		c.Handle(500, fmt.Sprintf("GetTags '%s'", c.Repo.Repository.RepoPath()), err)
		return
	}

	releases, err := models.GetPublishedReleasesByRepoID(c.Repo.Repository.ID, tagsResult.Tags...)
	if err != nil {
		c.Handle(500, "GetPublishedReleasesByRepoID", err)
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
				c.Handle(500, "LoadAttributes", err)
				return
			}

			if err := calReleaseNumCommitsBehind(c.Repo, r, countCache); err != nil {
				c.Handle(500, "calReleaseNumCommitsBehind", err)
				return
			}

			r.Note = string(markup.Markdown(r.Note, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))
			results[i] = r
			break
		}

		// No published release matches this tag
		if results[i] == nil {
			commit, err := c.Repo.GitRepo.GetTagCommit(rawTag)
			if err != nil {
				c.Handle(500, "GetTagCommit", err)
				return
			}

			results[i] = &models.Release{
				Title:   rawTag,
				TagName: rawTag,
				Sha1:    commit.ID.String(),
			}

			results[i].NumCommits, err = commit.CommitsCount()
			if err != nil {
				c.Handle(500, "CommitsCount", err)
				return
			}
			results[i].NumCommitsBehind = c.Repo.CommitsCount - results[i].NumCommits
		}
	}
	models.SortReleases(results)

	// Only show drafts if user is viewing the latest page
	var drafts []*models.Release
	if tagsResult.HasLatest {
		drafts, err = models.GetDraftReleasesByRepoID(c.Repo.Repository.ID)
		if err != nil {
			c.Handle(500, "GetDraftReleasesByRepoID", err)
			return
		}

		for _, r := range drafts {
			if err = r.LoadAttributes(); err != nil {
				c.Handle(500, "LoadAttributes", err)
				return
			}

			if err := calReleaseNumCommitsBehind(c.Repo, r, countCache); err != nil {
				c.Handle(500, "calReleaseNumCommitsBehind", err)
				return
			}

			r.Note = string(markup.Markdown(r.Note, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))
		}

		if len(drafts) > 0 {
			results = append(drafts, results...)
		}
	}

	c.Data["Releases"] = results
	c.Data["HasPrevious"] = !tagsResult.HasLatest
	c.Data["ReachEnd"] = tagsResult.ReachEnd
	c.Data["PreviousAfter"] = tagsResult.PreviousAfter
	if len(results) > 0 {
		c.Data["NextAfter"] = results[len(results)-1].TagName
	}
	c.HTML(200, RELEASES)
}

func renderReleaseAttachmentSettings(c *context.Context) {
	c.Data["RequireDropzone"] = true
	c.Data["IsAttachmentEnabled"] = setting.Release.Attachment.Enabled
	c.Data["AttachmentAllowedTypes"] = strings.Join(setting.Release.Attachment.AllowedTypes, ",")
	c.Data["AttachmentMaxSize"] = setting.Release.Attachment.MaxSize
	c.Data["AttachmentMaxFiles"] = setting.Release.Attachment.MaxFiles
}

func NewRelease(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.release.new_release")
	c.Data["PageIsReleaseList"] = true
	c.Data["tag_target"] = c.Repo.Repository.DefaultBranch
	renderReleaseAttachmentSettings(c)
	c.HTML(200, RELEASE_NEW)
}

func NewReleasePost(c *context.Context, f form.NewRelease) {
	c.Data["Title"] = c.Tr("repo.release.new_release")
	c.Data["PageIsReleaseList"] = true
	renderReleaseAttachmentSettings(c)

	if c.HasError() {
		c.HTML(200, RELEASE_NEW)
		return
	}

	if !c.Repo.GitRepo.IsBranchExist(f.Target) {
		c.RenderWithErr(c.Tr("form.target_branch_not_exist"), RELEASE_NEW, &f)
		return
	}

	// Use current time if tag not yet exist, otherwise get time from Git
	var tagCreatedUnix int64
	tag, err := c.Repo.GitRepo.GetTag(f.TagName)
	if err == nil {
		commit, err := tag.Commit()
		if err == nil {
			tagCreatedUnix = commit.Author.When.Unix()
		}
	}

	commit, err := c.Repo.GitRepo.GetBranchCommit(f.Target)
	if err != nil {
		c.Handle(500, "GetBranchCommit", err)
		return
	}

	commitsCount, err := commit.CommitsCount()
	if err != nil {
		c.Handle(500, "CommitsCount", err)
		return
	}

	var attachments []string
	if setting.Release.Attachment.Enabled {
		attachments = f.Files
	}

	rel := &models.Release{
		RepoID:       c.Repo.Repository.ID,
		PublisherID:  c.User.ID,
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
	if err = models.NewRelease(c.Repo.GitRepo, rel, attachments); err != nil {
		c.Data["Err_TagName"] = true
		switch {
		case models.IsErrReleaseAlreadyExist(err):
			c.RenderWithErr(c.Tr("repo.release.tag_name_already_exist"), RELEASE_NEW, &f)
		case models.IsErrInvalidTagName(err):
			c.RenderWithErr(c.Tr("repo.release.tag_name_invalid"), RELEASE_NEW, &f)
		default:
			c.Handle(500, "NewRelease", err)
		}
		return
	}
	log.Trace("Release created: %s/%s:%s", c.User.LowerName, c.Repo.Repository.Name, f.TagName)

	c.Redirect(c.Repo.RepoLink + "/releases")
}

func EditRelease(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.release.edit_release")
	c.Data["PageIsReleaseList"] = true
	c.Data["PageIsEditRelease"] = true
	renderReleaseAttachmentSettings(c)

	tagName := c.Params("*")
	rel, err := models.GetRelease(c.Repo.Repository.ID, tagName)
	if err != nil {
		if models.IsErrReleaseNotExist(err) {
			c.Handle(404, "GetRelease", err)
		} else {
			c.Handle(500, "GetRelease", err)
		}
		return
	}
	c.Data["ID"] = rel.ID
	c.Data["tag_name"] = rel.TagName
	c.Data["tag_target"] = rel.Target
	c.Data["title"] = rel.Title
	c.Data["content"] = rel.Note
	c.Data["attachments"] = rel.Attachments
	c.Data["prerelease"] = rel.IsPrerelease
	c.Data["IsDraft"] = rel.IsDraft

	c.HTML(200, RELEASE_NEW)
}

func EditReleasePost(c *context.Context, f form.EditRelease) {
	c.Data["Title"] = c.Tr("repo.release.edit_release")
	c.Data["PageIsReleaseList"] = true
	c.Data["PageIsEditRelease"] = true
	renderReleaseAttachmentSettings(c)

	tagName := c.Params("*")
	rel, err := models.GetRelease(c.Repo.Repository.ID, tagName)
	if err != nil {
		if models.IsErrReleaseNotExist(err) {
			c.Handle(404, "GetRelease", err)
		} else {
			c.Handle(500, "GetRelease", err)
		}
		return
	}
	c.Data["tag_name"] = rel.TagName
	c.Data["tag_target"] = rel.Target
	c.Data["title"] = rel.Title
	c.Data["content"] = rel.Note
	c.Data["attachments"] = rel.Attachments
	c.Data["prerelease"] = rel.IsPrerelease
	c.Data["IsDraft"] = rel.IsDraft

	if c.HasError() {
		c.HTML(200, RELEASE_NEW)
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
	if err = models.UpdateRelease(c.User, c.Repo.GitRepo, rel, isPublish, attachments); err != nil {
		c.Handle(500, "UpdateRelease", err)
		return
	}
	c.Redirect(c.Repo.RepoLink + "/releases")
}

func UploadReleaseAttachment(c *context.Context) {
	if !setting.Release.Attachment.Enabled {
		c.NotFound()
		return
	}
	uploadAttachment(c, setting.Release.Attachment.AllowedTypes)
}

func DeleteRelease(c *context.Context) {
	if err := models.DeleteReleaseOfRepoByID(c.Repo.Repository.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteReleaseByID: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.release.deletion_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/releases",
	})
}
