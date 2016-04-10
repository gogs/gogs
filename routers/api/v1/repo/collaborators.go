// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/context"
)

func AddCollaborator(ctx *context.APIContext, form api.AddCollaboratorOption) {
	collaborator, err := models.GetUserByName(ctx.Params(":collaborator"))

	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Error(422, "", err)
		} else {
			ctx.Error(500, "GetUserByName", err)
		}
		return
	}

	if err := ctx.Repo.Repository.AddCollaborator(collaborator); err != nil {
		ctx.Error(500, "AddCollaborator", err)
		return
	}

	mode := models.ACCESS_MODE_WRITE
	if form.Permission != nil && *form.Permission == "pull" {
		mode = models.ACCESS_MODE_READ
	} else if form.Permission != nil && *form.Permission == "push" {
		mode = models.ACCESS_MODE_WRITE
	} else if form.Permission != nil && *form.Permission == "admin" {
		mode = models.ACCESS_MODE_ADMIN
	} else if form.Permission != nil {
		ctx.Error(500, "Permission", "Invalid permission type")
		return
	}
	if err := ctx.Repo.Repository.ChangeCollaborationAccessMode(collaborator.Id, mode); err != nil {
		ctx.Error(500, "ChangeCollaborationAccessMode", err)
		return
	}

	ctx.Status(204)
	return
}
