// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
)

func ListIssueLabels(c *context.APIContext) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}

	apiLabels := make([]*api.Label, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSONSuccess(&apiLabels)
}

func AddIssueLabels(c *context.APIContext, form api.IssueLabelsOption) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}

	labels, err := models.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.ServerError("GetLabelsInRepoByIDs", err)
		return
	}

	if err = issue.AddLabels(c.User, labels); err != nil {
		c.ServerError("AddLabels", err)
		return
	}

	labels, err = models.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.ServerError("GetLabelsByIssueID", err)
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSONSuccess(&apiLabels)
}

func DeleteIssueLabel(c *context.APIContext) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}

	label, err := models.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrLabelNotExist(err) {
			c.Error(http.StatusUnprocessableEntity, "", err)
		} else {
			c.ServerError("GetLabelInRepoByID", err)
		}
		return
	}

	if err := models.DeleteIssueLabel(issue, label); err != nil {
		c.ServerError("DeleteIssueLabel", err)
		return
	}

	c.NoContent()
}

func ReplaceIssueLabels(c *context.APIContext, form api.IssueLabelsOption) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}

	labels, err := models.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.ServerError("GetLabelsInRepoByIDs", err)
		return
	}

	if err := issue.ReplaceLabels(labels); err != nil {
		c.ServerError("ReplaceLabels", err)
		return
	}

	labels, err = models.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.ServerError("GetLabelsByIssueID", err)
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSONSuccess(&apiLabels)
}

func ClearIssueLabels(c *context.APIContext) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}

	if err := issue.ClearLabels(c.User); err != nil {
		c.ServerError("ClearLabels", err)
		return
	}

	c.NoContent()
}
