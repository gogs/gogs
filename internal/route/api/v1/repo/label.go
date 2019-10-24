// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	"github.com/unknwon/com"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

func ListLabels(c *context.APIContext) {
	labels, err := db.GetLabelsByRepoID(c.Repo.Repository.ID)
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
	var label *db.Label
	var err error
	idStr := c.Params(":id")
	if id := com.StrTo(idStr).MustInt64(); id > 0 {
		label, err = db.GetLabelOfRepoByID(c.Repo.Repository.ID, id)
	} else {
		label, err = db.GetLabelOfRepoByName(c.Repo.Repository.ID, idStr)
	}
	if err != nil {
		c.NotFoundOrServerError("GetLabel", db.IsErrLabelNotExist, err)
		return
	}

	c.JSONSuccess(label.APIFormat())
}

func CreateLabel(c *context.APIContext, form api.CreateLabelOption) {
	label := &db.Label{
		Name:   form.Name,
		Color:  form.Color,
		RepoID: c.Repo.Repository.ID,
	}
	if err := db.NewLabels(label); err != nil {
		c.ServerError("NewLabel", err)
		return
	}
	c.JSON(http.StatusCreated, label.APIFormat())
}

func EditLabel(c *context.APIContext, form api.EditLabelOption) {
	label, err := db.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetLabelOfRepoByID", db.IsErrLabelNotExist, err)
		return
	}

	if form.Name != nil {
		label.Name = *form.Name
	}
	if form.Color != nil {
		label.Color = *form.Color
	}
	if err := db.UpdateLabel(label); err != nil {
		c.ServerError("UpdateLabel", err)
		return
	}
	c.JSONSuccess(label.APIFormat())
}

func DeleteLabel(c *context.APIContext) {
	if err := db.DeleteLabel(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.ServerError("DeleteLabel", err)
		return
	}

	c.NoContent()
}
