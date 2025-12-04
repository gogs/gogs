// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func ListIssueLabels(c *context.APIContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	apiLabels := make([]*api.Label, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSONSuccess(&apiLabels)
}

func AddIssueLabels(c *context.APIContext, form api.IssueLabelsOption) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	labels, err := database.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(err, "get labels in repository by IDs")
		return
	}

	if err = issue.AddLabels(c.User, labels); err != nil {
		c.Error(err, "add labels")
		return
	}

	labels, err = database.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.Error(err, "get labels by issue ID")
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSONSuccess(&apiLabels)
}

func DeleteIssueLabel(c *context.APIContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	label, err := database.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if database.IsErrLabelNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get label of repository by ID")
		}
		return
	}

	if err := database.DeleteIssueLabel(issue, label); err != nil {
		c.Error(err, "delete issue label")
		return
	}

	c.NoContent()
}

func ReplaceIssueLabels(c *context.APIContext, form api.IssueLabelsOption) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	labels, err := database.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(err, "get labels in repository by IDs")
		return
	}

	if err := issue.ReplaceLabels(labels); err != nil {
		c.Error(err, "replace labels")
		return
	}

	labels, err = database.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.Error(err, "get labels by issue ID")
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSONSuccess(&apiLabels)
}

func ClearIssueLabels(c *context.APIContext) {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}

	if err := issue.ClearLabels(c.User); err != nil {
		c.Error(err, "clear labels")
		return
	}

	c.NoContent()
}
