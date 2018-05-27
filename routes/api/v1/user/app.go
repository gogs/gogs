// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
)

// https://github.com/gogs/go-gogs-client/wiki/Users#list-access-tokens-for-a-user
func ListAccessTokens(c *context.APIContext) {
	tokens, err := models.ListAccessTokens(c.User.ID)
	if err != nil {
		c.Error(500, "ListAccessTokens", err)
		return
	}

	apiTokens := make([]*api.AccessToken, len(tokens))
	for i := range tokens {
		apiTokens[i] = &api.AccessToken{tokens[i].Name, tokens[i].Sha1}
	}
	c.JSON(200, &apiTokens)
}

// https://github.com/gogs/go-gogs-client/wiki/Users#create-a-access-token
func CreateAccessToken(c *context.APIContext, form api.CreateAccessTokenOption) {
	t := &models.AccessToken{
		UID:  c.User.ID,
		Name: form.Name,
	}
	if err := models.NewAccessToken(t); err != nil {
		c.Error(500, "NewAccessToken", err)
		return
	}
	c.JSON(201, &api.AccessToken{t.Name, t.Sha1})
}
