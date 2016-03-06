// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

func AddCollaborator(ctx *middleware.Context) {
	collaborator, err := models.GetUserByName(ctx.Params(":collaborator"))

	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.APIError(422, "", err)
		} else {
			ctx.APIError(500, "GetUserByName", err)
		}
		return
	}

	if err := ctx.Repo.Repository.AddCollaborator(collaborator); err != nil {
		ctx.APIError(500, "AddCollaborator", err)
		return
	}

	ctx.Status(204)
	return
}
