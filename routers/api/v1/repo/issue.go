// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/setting"
)

func listIssues(ctx *context.APIContext, opts *models.IssuesOptions) {
	issues, err := models.Issues(opts)
	if err != nil {
		ctx.Error(500, "Issues", err)
		return
	}

	count, err := models.IssuesCount(opts)
	if err != nil {
		ctx.Error(500, "IssuesCount", err)
		return
	}

	// FIXME: use IssueList to improve performance.
	apiIssues := make([]*api.Issue, len(issues))
	for i := range issues {
		if err = issues[i].LoadAttributes(); err != nil {
			ctx.Error(500, "LoadAttributes", err)
			return
		}
		apiIssues[i] = issues[i].APIFormat()
	}

	ctx.SetLinkHeader(int(count), setting.UI.IssuePagingNum)
	ctx.JSON(200, &apiIssues)
}

func ListUserIssues(ctx *context.APIContext) {
	opts := models.IssuesOptions{
		AssigneeID: ctx.User.ID,
		Page:       ctx.QueryInt("page"),
	}

	listIssues(ctx, &opts)
}

func ListIssues(ctx *context.APIContext) {
	opts := models.IssuesOptions{
		RepoID: ctx.Repo.Repository.ID,
		Page:   ctx.QueryInt("page"),
	}

	listIssues(ctx, &opts)
}

func GetIssue(ctx *context.APIContext) {
	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if errors.IsIssueNotExist(err) {
			ctx.Status(404)
		} else {
			ctx.Error(500, "GetIssueByIndex", err)
		}
		return
	}
	ctx.JSON(200, issue.APIFormat())
}

func CreateIssue(ctx *context.APIContext, form api.CreateIssueOption) {
	issue := &models.Issue{
		RepoID:   ctx.Repo.Repository.ID,
		Title:    form.Title,
		PosterID: ctx.User.ID,
		Poster:   ctx.User,
		Content:  form.Body,
	}

	if ctx.Repo.IsWriter() {
		if len(form.Assignee) > 0 {
			assignee, err := models.GetUserByName(form.Assignee)
			if err != nil {
				if errors.IsUserNotExist(err) {
					ctx.Error(422, "", fmt.Sprintf("Assignee does not exist: [name: %s]", form.Assignee))
				} else {
					ctx.Error(500, "GetUserByName", err)
				}
				return
			}
			issue.AssigneeID = assignee.ID
		}
		issue.MilestoneID = form.Milestone
	} else {
		form.Labels = nil
	}

	if err := models.NewIssue(ctx.Repo.Repository, issue, form.Labels, nil); err != nil {
		ctx.Error(500, "NewIssue", err)
		return
	}

	if form.Closed {
		if err := issue.ChangeStatus(ctx.User, ctx.Repo.Repository, true); err != nil {
			ctx.Error(500, "ChangeStatus", err)
			return
		}
	}

	// Refetch from database to assign some automatic values
	var err error
	issue, err = models.GetIssueByID(issue.ID)
	if err != nil {
		ctx.Error(500, "GetIssueByID", err)
		return
	}
	ctx.JSON(201, issue.APIFormat())
}

func EditIssue(ctx *context.APIContext, form api.EditIssueOption) {
	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if errors.IsIssueNotExist(err) {
			ctx.Status(404)
		} else {
			ctx.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	if !issue.IsPoster(ctx.User.ID) && !ctx.Repo.IsWriter() {
		ctx.Status(403)
		return
	}

	if len(form.Title) > 0 {
		issue.Title = form.Title
	}
	if form.Body != nil {
		issue.Content = *form.Body
	}

	if ctx.Repo.IsWriter() && form.Assignee != nil &&
		(issue.Assignee == nil || issue.Assignee.LowerName != strings.ToLower(*form.Assignee)) {
		if len(*form.Assignee) == 0 {
			issue.AssigneeID = 0
		} else {
			assignee, err := models.GetUserByName(*form.Assignee)
			if err != nil {
				if errors.IsUserNotExist(err) {
					ctx.Error(422, "", fmt.Sprintf("assignee does not exist: [name: %s]", *form.Assignee))
				} else {
					ctx.Error(500, "GetUserByName", err)
				}
				return
			}
			issue.AssigneeID = assignee.ID
		}

		if err = models.UpdateIssueUserByAssignee(issue); err != nil {
			ctx.Error(500, "UpdateIssueUserByAssignee", err)
			return
		}
	}
	if ctx.Repo.IsWriter() && form.Milestone != nil &&
		issue.MilestoneID != *form.Milestone {
		oldMilestoneID := issue.MilestoneID
		issue.MilestoneID = *form.Milestone
		if err = models.ChangeMilestoneAssign(ctx.User, issue, oldMilestoneID); err != nil {
			ctx.Error(500, "ChangeMilestoneAssign", err)
			return
		}
	}

	if err = models.UpdateIssue(issue); err != nil {
		ctx.Error(500, "UpdateIssue", err)
		return
	}
	if form.State != nil {
		if err = issue.ChangeStatus(ctx.User, ctx.Repo.Repository, api.STATE_CLOSED == api.StateType(*form.State)); err != nil {
			ctx.Error(500, "ChangeStatus", err)
			return
		}
	}

	// Refetch from database to assign some automatic values
	issue, err = models.GetIssueByID(issue.ID)
	if err != nil {
		ctx.Error(500, "GetIssueByID", err)
		return
	}
	ctx.JSON(201, issue.APIFormat())
}
