// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"
	"time"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
)

func ListMilestones(c *context.APIContext) {
	milestones, err := models.GetMilestonesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.ServerError("GetMilestonesByRepoID", err)
		return
	}

	apiMilestones := make([]*api.Milestone, len(milestones))
	for i := range milestones {
		apiMilestones[i] = milestones[i].APIFormat()
	}
	c.JSONSuccess(&apiMilestones)
}

func GetMilestone(c *context.APIContext) {
	milestone, err := models.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetMilestoneByRepoID", models.IsErrMilestoneNotExist, err)
		return
	}
	c.JSONSuccess(milestone.APIFormat())
}

func CreateMilestone(c *context.APIContext, form api.CreateMilestoneOption) {
	if form.Deadline == nil {
		defaultDeadline, _ := time.ParseInLocation("2006-01-02", "9999-12-31", time.Local)
		form.Deadline = &defaultDeadline
	}

	milestone := &models.Milestone{
		RepoID:   c.Repo.Repository.ID,
		Name:     form.Title,
		Content:  form.Description,
		Deadline: *form.Deadline,
	}

	if err := models.NewMilestone(milestone); err != nil {
		c.ServerError("NewMilestone", err)
		return
	}
	c.JSON(http.StatusCreated, milestone.APIFormat())
}

func EditMilestone(c *context.APIContext, form api.EditMilestoneOption) {
	milestone, err := models.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetMilestoneByRepoID", models.IsErrMilestoneNotExist, err)
		return
	}

	if len(form.Title) > 0 {
		milestone.Name = form.Title
	}
	if form.Description != nil {
		milestone.Content = *form.Description
	}
	if form.Deadline != nil && !form.Deadline.IsZero() {
		milestone.Deadline = *form.Deadline
	}

	if form.State != nil {
		if err = milestone.ChangeStatus(api.STATE_CLOSED == api.StateType(*form.State)); err != nil {
			c.ServerError("ChangeStatus", err)
			return
		}
	} else if err = models.UpdateMilestone(milestone); err != nil {
		c.ServerError("UpdateMilestone", err)
		return
	}

	c.JSONSuccess(milestone.APIFormat())
}

func DeleteMilestone(c *context.APIContext) {
	if err := models.DeleteMilestoneOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.ServerError("DeleteMilestoneByRepoID", err)
		return
	}
	c.NoContent()
}
