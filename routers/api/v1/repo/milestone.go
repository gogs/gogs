// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/routers/api/v1/convert"
	"time"
)

func ListMilestones(ctx *context.APIContext) {
	milestones, err := models.GetAllRepoMilestones(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Error(500, "GetAllRepoMilestones", err)
		return
	}

	apiMilestones := make([]*api.Milestone, len(milestones))
	for i := range milestones {
		apiMilestones[i] = convert.ToMilestone(milestones[i])
	}
	ctx.JSON(200, &apiMilestones)
}

func GetMilestone(ctx *context.APIContext) {
	milestone, err := models.GetRepoMilestoneByID(ctx.Repo.Repository.ID, ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Status(404)
		} else {
			ctx.Error(500, "GetRepoMilestoneByID", err)
		}
		return
	}
	ctx.JSON(200, convert.ToMilestone(milestone))
}

func CreateMilestone(ctx *context.APIContext, form api.CreateMilestoneOption) {
	if !ctx.Repo.IsWriter() {
		ctx.Status(403)
		return
	}

	if form.Deadline == nil {
		defaultDeadline, _ := time.ParseInLocation("2006-01-02", "9999-12-31", time.Local)
		form.Deadline = &defaultDeadline
	}

	milestone := &models.Milestone{
		RepoID:   ctx.Repo.Repository.ID,
		Name:     form.Title,
		Content:  form.Description,
		Deadline: *form.Deadline,
	}

	if err := models.NewMilestone(milestone); err != nil {
		ctx.Error(500, "NewMilestone", err)
		return
	}
	ctx.JSON(201, convert.ToMilestone(milestone))
}

func EditMilestone(ctx *context.APIContext, form api.EditMilestoneOption) {
	if !ctx.Repo.IsWriter() {
		ctx.Status(403)
		return
	}

	milestone, err := models.GetRepoMilestoneByID(ctx.Repo.Repository.ID, ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Status(404)
		} else {
			ctx.Error(500, "GetRepoMilestoneByID", err)
		}
		return
	}

	if len(form.Title) > 0 {
		milestone.Name = form.Title
	}
	if len(form.Description) > 0 {
		milestone.Content = form.Description
	}
	if !form.Deadline.IsZero() {
		milestone.Deadline = *form.Deadline
	}
	if err := models.UpdateMilestone(milestone); err != nil {
		ctx.Handle(500, "UpdateMilestone", err)
		return
	}
	ctx.JSON(200, convert.ToMilestone(milestone))
}

func DeleteMilestone(ctx *context.APIContext) {
	if !ctx.Repo.IsWriter() {
		ctx.Status(403)
		return
	}

	if err := models.DeleteMilestoneByID(ctx.ParamsInt64(":id")); err != nil {
		ctx.Error(500, "DeleteMilestoneByID", err)
		return
	}
	ctx.Status(204)
}

func ChangeMilestoneStatus(ctx *context.APIContext) {
	if !ctx.Repo.IsWriter() {
		ctx.Status(403)
		return
	}

	m, err := models.GetMilestoneByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Handle(404, "GetMilestoneByID", err)
		} else {
			ctx.Handle(500, "GetMilestoneByID", err)
		}
		return
	}

	switch ctx.Params(":action") {
	case "open":
		if m.IsClosed {
			if err = models.ChangeMilestoneStatus(m, false); err != nil {
				ctx.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		ctx.JSON(200, convert.ToMilestone(m))
	case "close":
		if !m.IsClosed {
			m.ClosedDate = time.Now()
			if err = models.ChangeMilestoneStatus(m, true); err != nil {
				ctx.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		ctx.JSON(200, convert.ToMilestone(m))
	default:
		ctx.Status(400)
	}
}
