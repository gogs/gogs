// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"
	"time"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func ListMilestones(c *context.APIContext) {
	milestones, err := database.GetMilestonesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get milestones by repository ID")
		return
	}

	apiMilestones := make([]*api.Milestone, len(milestones))
	for i := range milestones {
		apiMilestones[i] = milestones[i].APIFormat()
	}
	c.JSONSuccess(&apiMilestones)
}

func GetMilestone(c *context.APIContext) {
	milestone, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get milestone by repository ID")
		return
	}
	c.JSONSuccess(milestone.APIFormat())
}

func CreateMilestone(c *context.APIContext, form api.CreateMilestoneOption) {
	if form.Deadline == nil {
		defaultDeadline, _ := time.ParseInLocation("2006-01-02", "9999-12-31", time.Local)
		form.Deadline = &defaultDeadline
	}

	milestone := &database.Milestone{
		RepoID:   c.Repo.Repository.ID,
		Name:     form.Title,
		Content:  form.Description,
		Deadline: *form.Deadline,
	}

	if err := database.NewMilestone(milestone); err != nil {
		c.Error(err, "new milestone")
		return
	}
	c.JSON(http.StatusCreated, milestone.APIFormat())
}

func EditMilestone(c *context.APIContext, form api.EditMilestoneOption) {
	milestone, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get milestone by repository ID")
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
			c.Error(err, "change status")
			return
		}
	} else if err = database.UpdateMilestone(milestone); err != nil {
		c.Error(err, "update milestone")
		return
	}

	c.JSONSuccess(milestone.APIFormat())
}

func DeleteMilestone(c *context.APIContext) {
	if err := database.DeleteMilestoneOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.Error(err, "delete milestone of repository by ID")
		return
	}
	c.NoContent()
}
