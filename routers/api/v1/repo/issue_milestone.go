// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/routers/api/v1/convert"
)

func GetIssueMilestone(ctx *context.APIContext) {
	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Status(404)
		} else {
			ctx.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	apiMilestone := convert.ToMilestone(issue.Milestone)
	ctx.JSON(200, &apiMilestone)
}

func SetIssueMilestone(ctx *context.APIContext, form api.SetIssueMilestoneOption) {
	if !ctx.Repo.IsWriter() {
		ctx.Status(403)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Status(404)
		} else {
			ctx.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	oldMid := issue.MilestoneID
	if oldMid != form.ID {
		issue.MilestoneID = form.ID
		if err = models.ChangeMilestoneAssign(oldMid, issue); err != nil {
			ctx.Error(500, "ChangeMilestoneAssign", err)
			return
		}
	}

	// Refresh issue to return updated milestone
	issue, err = models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Status(404)
		} else {
			ctx.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	apiMilestone := convert.ToMilestone(issue.Milestone)
	ctx.JSON(200, &apiMilestone)
}

func DeleteIssueMilestone(ctx *context.APIContext) {
	form := api.SetIssueMilestoneOption{
		ID: 0,
	}

	SetIssueMilestone(ctx, form)
}
