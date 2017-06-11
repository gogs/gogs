// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/Unknwon/com"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
)

func ListLabels(c *context.APIContext) {
	labels, err := models.GetLabelsByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(500, "GetLabelsByRepoID", err)
		return
	}

	apiLabels := make([]*api.Label, len(labels))
	for i := range labels {
		apiLabels[i] = labels[i].APIFormat()
	}
	c.JSON(200, &apiLabels)
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
		if models.IsErrLabelNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetLabelByRepoID", err)
		}
		return
	}

	c.JSON(200, label.APIFormat())
}

func CreateLabel(c *context.APIContext, form api.CreateLabelOption) {
	if !c.Repo.IsWriter() {
		c.Status(403)
		return
	}

	label := &models.Label{
		Name:   form.Name,
		Color:  form.Color,
		RepoID: c.Repo.Repository.ID,
	}
	if err := models.NewLabels(label); err != nil {
		c.Error(500, "NewLabel", err)
		return
	}
	c.JSON(201, label.APIFormat())
}

func EditLabel(c *context.APIContext, form api.EditLabelOption) {
	if !c.Repo.IsWriter() {
		c.Status(403)
		return
	}

	label, err := models.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrLabelNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetLabelByRepoID", err)
		}
		return
	}

	if form.Name != nil {
		label.Name = *form.Name
	}
	if form.Color != nil {
		label.Color = *form.Color
	}
	if err := models.UpdateLabel(label); err != nil {
		c.Handle(500, "UpdateLabel", err)
		return
	}
	c.JSON(200, label.APIFormat())
}

func DeleteLabel(c *context.APIContext) {
	if !c.Repo.IsWriter() {
		c.Status(403)
		return
	}

	if err := models.DeleteLabel(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.Error(500, "DeleteLabel", err)
		return
	}

	c.Status(204)
}
