// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"strings"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/setting"
)

func listIssues(c *context.APIContext, opts *models.IssuesOptions) {
	issues, err := models.Issues(opts)
	if err != nil {
		c.ServerError("Issues", err)
		return
	}

	count, err := models.IssuesCount(opts)
	if err != nil {
		c.ServerError("IssuesCount", err)
		return
	}

	// FIXME: use IssueList to improve performance.
	apiIssues := make([]*api.Issue, len(issues))
	for i := range issues {
		if err = issues[i].LoadAttributes(); err != nil {
			c.ServerError("LoadAttributes", err)
			return
		}
		apiIssues[i] = issues[i].APIFormat()
	}

	c.SetLinkHeader(int(count), setting.UI.IssuePagingNum)
	c.JSONSuccess(&apiIssues)
}

func ListUserIssues(c *context.APIContext) {
	opts := models.IssuesOptions{
		AssigneeID: c.User.ID,
		Page:       c.QueryInt("page"),
		IsClosed:   api.StateType(c.Query("state")) == api.STATE_CLOSED,
	}

	listIssues(c, &opts)
}

func ListIssues(c *context.APIContext) {
	opts := models.IssuesOptions{
		RepoID:   c.Repo.Repository.ID,
		Page:     c.QueryInt("page"),
		IsClosed: api.StateType(c.Query("state")) == api.STATE_CLOSED,
	}

	listIssues(c, &opts)
}

func GetIssue(c *context.APIContext) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}
	c.JSONSuccess(issue.APIFormat())
}

func CreateIssue(c *context.APIContext, form api.CreateIssueOption) {
	issue := &models.Issue{
		RepoID:   c.Repo.Repository.ID,
		Title:    form.Title,
		PosterID: c.User.ID,
		Poster:   c.User,
		Content:  form.Body,
	}

	if c.Repo.IsWriter() {
		if len(form.Assignee) > 0 {
			assignee, err := models.GetUserByName(form.Assignee)
			if err != nil {
				if errors.IsUserNotExist(err) {
					c.Error(http.StatusUnprocessableEntity, "", fmt.Sprintf("assignee does not exist: [name: %s]", form.Assignee))
				} else {
					c.ServerError("GetUserByName", err)
				}
				return
			}
			issue.AssigneeID = assignee.ID
		}
		issue.MilestoneID = form.Milestone
	} else {
		form.Labels = nil
	}

	if err := models.NewIssue(c.Repo.Repository, issue, form.Labels, nil); err != nil {
		c.ServerError("NewIssue", err)
		return
	}

	if form.Closed {
		if err := issue.ChangeStatus(c.User, c.Repo.Repository, true); err != nil {
			c.ServerError("ChangeStatus", err)
			return
		}
	}

	// Refetch from database to assign some automatic values
	var err error
	issue, err = models.GetIssueByID(issue.ID)
	if err != nil {
		c.ServerError("GetIssueByID", err)
		return
	}
	c.JSON(http.StatusCreated, issue.APIFormat())
}

func EditIssue(c *context.APIContext, form api.EditIssueOption) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}

	if !issue.IsPoster(c.User.ID) && !c.Repo.IsWriter() {
		c.Status(http.StatusForbidden)
		return
	}

	if len(form.Title) > 0 {
		issue.Title = form.Title
	}
	if form.Body != nil {
		issue.Content = *form.Body
	}

	if c.Repo.IsWriter() && form.Assignee != nil &&
		(issue.Assignee == nil || issue.Assignee.LowerName != strings.ToLower(*form.Assignee)) {
		if len(*form.Assignee) == 0 {
			issue.AssigneeID = 0
		} else {
			assignee, err := models.GetUserByName(*form.Assignee)
			if err != nil {
				if errors.IsUserNotExist(err) {
					c.Error(http.StatusUnprocessableEntity, "", fmt.Sprintf("assignee does not exist: [name: %s]", *form.Assignee))
				} else {
					c.ServerError("GetUserByName", err)
				}
				return
			}
			issue.AssigneeID = assignee.ID
		}

		if err = models.UpdateIssueUserByAssignee(issue); err != nil {
			c.ServerError("UpdateIssueUserByAssignee", err)
			return
		}
	}
	if c.Repo.IsWriter() && form.Milestone != nil &&
		issue.MilestoneID != *form.Milestone {
		oldMilestoneID := issue.MilestoneID
		issue.MilestoneID = *form.Milestone
		if err = models.ChangeMilestoneAssign(c.User, issue, oldMilestoneID); err != nil {
			c.ServerError("ChangeMilestoneAssign", err)
			return
		}
	}

	if err = models.UpdateIssue(issue); err != nil {
		c.ServerError("UpdateIssue", err)
		return
	}
	if form.State != nil {
		if err = issue.ChangeStatus(c.User, c.Repo.Repository, api.STATE_CLOSED == api.StateType(*form.State)); err != nil {
			c.ServerError("ChangeStatus", err)
			return
		}
	}

	// Refetch from database to assign some automatic values
	issue, err = models.GetIssueByID(issue.ID)
	if err != nil {
		c.ServerError("GetIssueByID", err)
		return
	}
	c.JSON(http.StatusCreated, issue.APIFormat())
}
