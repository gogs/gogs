// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
)

func ListIssueLabels(c *context.APIContext) {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		if errors.IsIssueNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	apiLabels := make([]*api.Label, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSON(200, &apiLabels)
}

func AddIssueLabels(c *context.APIContext, form api.IssueLabelsOption) {
	if !c.Repo.IsWriter() {
		c.Status(403)
		return
	}

	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		if errors.IsIssueNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	labels, err := models.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(500, "GetLabelsInRepoByIDs", err)
		return
	}

	if err = issue.AddLabels(c.User, labels); err != nil {
		c.Error(500, "AddLabels", err)
		return
	}

	labels, err = models.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.Error(500, "GetLabelsByIssueID", err)
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSON(200, &apiLabels)
}

func DeleteIssueLabel(c *context.APIContext) {
	if !c.Repo.IsWriter() {
		c.Status(403)
		return
	}

	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		if errors.IsIssueNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	label, err := models.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrLabelNotExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "GetLabelInRepoByID", err)
		}
		return
	}

	if err := models.DeleteIssueLabel(issue, label); err != nil {
		c.Error(500, "DeleteIssueLabel", err)
		return
	}

	c.Status(204)
}

func ReplaceIssueLabels(c *context.APIContext, form api.IssueLabelsOption) {
	if !c.Repo.IsWriter() {
		c.Status(403)
		return
	}

	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		if errors.IsIssueNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	labels, err := models.GetLabelsInRepoByIDs(c.Repo.Repository.ID, form.Labels)
	if err != nil {
		c.Error(500, "GetLabelsInRepoByIDs", err)
		return
	}

	if err := issue.ReplaceLabels(labels); err != nil {
		c.Error(500, "ReplaceLabels", err)
		return
	}

	labels, err = models.GetLabelsByIssueID(issue.ID)
	if err != nil {
		c.Error(500, "GetLabelsByIssueID", err)
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}
	c.JSON(200, &apiLabels)
}

func ClearIssueLabels(c *context.APIContext) {
	if !c.Repo.IsWriter() {
		c.Status(403)
		return
	}

	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		if errors.IsIssueNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetIssueByIndex", err)
		}
		return
	}

	if err := issue.ClearLabels(c.User); err != nil {
		c.Error(500, "ClearLabels", err)
		return
	}

	c.Status(204)
}
