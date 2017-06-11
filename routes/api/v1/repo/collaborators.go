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

func ListCollaborators(c *context.APIContext) {
	collaborators, err := c.Repo.Repository.GetCollaborators()
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "GetCollaborators", err)
		}
		return
	}

	apiCollaborators := make([]*api.Collaborator, len(collaborators))
	for i := range collaborators {
		apiCollaborators[i] = collaborators[i].APIFormat()
	}
	c.JSON(200, &apiCollaborators)
}

func AddCollaborator(c *context.APIContext, form api.AddCollaboratorOption) {
	collaborator, err := models.GetUserByName(c.Params(":collaborator"))
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "GetUserByName", err)
		}
		return
	}

	if err := c.Repo.Repository.AddCollaborator(collaborator); err != nil {
		c.Error(500, "AddCollaborator", err)
		return
	}

	if form.Permission != nil {
		if err := c.Repo.Repository.ChangeCollaborationAccessMode(collaborator.ID, models.ParseAccessMode(*form.Permission)); err != nil {
			c.Error(500, "ChangeCollaborationAccessMode", err)
			return
		}
	}

	c.Status(204)
}

func IsCollaborator(c *context.APIContext) {
	collaborator, err := models.GetUserByName(c.Params(":collaborator"))
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "GetUserByName", err)
		}
		return
	}

	if !c.Repo.Repository.IsCollaborator(collaborator.ID) {
		c.Status(404)
	} else {
		c.Status(204)
	}
}

func DeleteCollaborator(c *context.APIContext) {
	collaborator, err := models.GetUserByName(c.Params(":collaborator"))
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "GetUserByName", err)
		}
		return
	}

	if err := c.Repo.Repository.DeleteCollaboration(collaborator.ID); err != nil {
		c.Error(500, "DeleteCollaboration", err)
		return
	}

	c.Status(204)
}
