// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
)

func ListAccessTokens(c *context.APIContext) {
	tokens, err := models.ListAccessTokens(c.User.ID)
	if err != nil {
		c.ServerError("ListAccessTokens", err)
		return
	}

	apiTokens := make([]*api.AccessToken, len(tokens))
	for i := range tokens {
		apiTokens[i] = &api.AccessToken{tokens[i].Name, tokens[i].Sha1}
	}
	c.JSONSuccess(&apiTokens)
}

func CreateAccessToken(c *context.APIContext, form api.CreateAccessTokenOption) {
	t := &models.AccessToken{
		UID:  c.User.ID,
		Name: form.Name,
	}
	if err := models.NewAccessToken(t); err != nil {
		if errors.IsAccessTokenNameAlreadyExist(err) {
			c.Error(http.StatusUnprocessableEntity, "", err)
		} else {
			c.ServerError("NewAccessToken", err)
		}
		return
	}
	c.JSON(http.StatusCreated, &api.AccessToken{t.Name, t.Sha1})
}
