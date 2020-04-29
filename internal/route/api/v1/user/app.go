// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

func ListAccessTokens(c *context.APIContext) {
	tokens, err := db.AccessTokens.List(c.User.ID)
	if err != nil {
		c.Error(err, "list access tokens")
		return
	}

	apiTokens := make([]*api.AccessToken, len(tokens))
	for i := range tokens {
		apiTokens[i] = &api.AccessToken{Name: tokens[i].Name, Sha1: tokens[i].Sha1}
	}
	c.JSONSuccess(&apiTokens)
}

func CreateAccessToken(c *context.APIContext, form api.CreateAccessTokenOption) {
	t, err := db.AccessTokens.Create(c.User.ID, form.Name)
	if err != nil {
		if db.IsErrAccessTokenAlreadyExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "new access token")
		}
		return
	}
	c.JSON(http.StatusCreated, &api.AccessToken{Name: t.Name, Sha1: t.Sha1})
}
