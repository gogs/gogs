// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
)

func ListAccessTokens(c *context.APIContext) {
	tokens, err := db.ListAccessTokens(c.User.ID)
	if err != nil {
		c.ServerError("ListAccessTokens", err)
		return
	}

	apiTokens := make([]*api.AccessToken, len(tokens))
	for i := range tokens {
		apiTokens[i] = &api.AccessToken{Name: tokens[i].Name, Sha1: tokens[i].Sha1}
	}
	c.JSONSuccess(&apiTokens)
}

func CreateAccessToken(c *context.APIContext, form api.CreateAccessTokenOption) {
	t := &db.AccessToken{
		UID:  c.User.ID,
		Name: form.Name,
	}
	if err := db.NewAccessToken(t); err != nil {
		if errors.IsAccessTokenNameAlreadyExist(err) {
			c.Error(http.StatusUnprocessableEntity, "", err)
		} else {
			c.ServerError("NewAccessToken", err)
		}
		return
	}
	c.JSON(http.StatusCreated, &api.AccessToken{Name: t.Name, Sha1: t.Sha1})
}
