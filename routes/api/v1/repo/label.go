// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	"github.com/Unknwon/com"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
)

func ListLabels(c *context.APIContext) {
	labels, err := models.GetLabelsByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.ServerError("GetLabelsByRepoID", err)
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = labels[i].APIFormat()
	}
	c.JSONSuccess(&apiLabels)
}

func GetLabel(c *context.APIContext) {
	var label *models.Label
	var err error
	idStr := c.Params(":id")
	if id := com.StrTo(idStr).MustInt64(); id > 0 {
		label, err = models.GetLabelOfRepoByID(c.Repo.Repository.ID, id)
	} else {
		label, err = models.GetLabelOfRepoByName(c.Repo.Repository.ID, idStr)
	}
	if err != nil {
		c.NotFoundOrServerError("GetLabel", models.IsErrLabelNotExist, err)
		return
	}

	c.JSONSuccess(label.APIFormat())
}

func CreateLabel(c *context.APIContext, form api.CreateLabelOption) {
	label := &models.Label{
		Name:   form.Name,
		Color:  form.Color,
		RepoID: c.Repo.Repository.ID,
	}
	if err := models.NewLabels(label); err != nil {
		c.ServerError("NewLabel", err)
		return
	}
	c.JSON(http.StatusCreated, label.APIFormat())
}

func EditLabel(c *context.APIContext, form api.EditLabelOption) {
	label, err := models.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetLabelOfRepoByID", models.IsErrLabelNotExist, err)
		return
	}

	if form.Name != nil {
		label.Name = *form.Name
	}
	if form.Color != nil {
		label.Color = *form.Color
	}
	if err := models.UpdateLabel(label); err != nil {
		c.ServerError("UpdateLabel", err)
		return
	}
	c.JSONSuccess(label.APIFormat())
}

func DeleteLabel(c *context.APIContext) {
	if err := models.DeleteLabel(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.ServerError("DeleteLabel", err)
		return
	}

	c.NoContent()
}
