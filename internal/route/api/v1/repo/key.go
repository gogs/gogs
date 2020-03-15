// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/route/api/v1/convert"
)

func composeDeployKeysAPILink(repoPath string) string {
	return conf.Server.ExternalURL + "api/v1/repos/" + repoPath + "/keys/"
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#list-deploy-keys
func ListDeployKeys(c *context.APIContext) {
	keys, err := db.ListDeployKeys(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "list deploy keys")
		return
	}

	apiLink := composeDeployKeysAPILink(c.Repo.Owner.Name + "/" + c.Repo.Repository.Name)
	apiKeys := make([]*api.DeployKey, len(keys))
	for i := range keys {
		if err = keys[i].GetContent(); err != nil {
			c.Error(err, "get content")
			return
		}
		apiKeys[i] = convert.ToDeployKey(apiLink, keys[i])
	}

	c.JSONSuccess(&apiKeys)
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#get-a-deploy-key
func GetDeployKey(c *context.APIContext) {
	key, err := db.GetDeployKeyByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get deploy key by ID")
		return
	}

	if err = key.GetContent(); err != nil {
		c.Error(err, "get content")
		return
	}

	apiLink := composeDeployKeysAPILink(c.Repo.Owner.Name + "/" + c.Repo.Repository.Name)
	c.JSONSuccess(convert.ToDeployKey(apiLink, key))
}

func HandleCheckKeyStringError(c *context.APIContext, err error) {
	if db.IsErrKeyUnableVerify(err) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Unable to verify key content"))
	} else {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Wrap(err, "Invalid key content: %v"))
	}
}

func HandleAddKeyError(c *context.APIContext, err error) {
	switch {
	case db.IsErrKeyAlreadyExist(err):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Key content has been used as non-deploy key"))
	case db.IsErrKeyNameAlreadyUsed(err):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Key title has been used"))
	default:
		c.Error(err, "add key")
	}
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#add-a-new-deploy-key
func CreateDeployKey(c *context.APIContext, form api.CreateKeyOption) {
	content, err := db.CheckPublicKeyString(form.Key)
	if err != nil {
		HandleCheckKeyStringError(c, err)
		return
	}

	key, err := db.AddDeployKey(c.Repo.Repository.ID, form.Title, content)
	if err != nil {
		HandleAddKeyError(c, err)
		return
	}

	key.Content = content
	apiLink := composeDeployKeysAPILink(c.Repo.Owner.Name + "/" + c.Repo.Repository.Name)
	c.JSON(http.StatusCreated, convert.ToDeployKey(apiLink, key))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#remove-a-deploy-key
func DeleteDeploykey(c *context.APIContext) {
	if err := db.DeleteDeployKey(c.User, c.ParamsInt64(":id")); err != nil {
		if db.IsErrKeyAccessDenied(err) {
			c.ErrorStatus(http.StatusForbidden, errors.New("You do not have access to this key"))
		} else {
			c.Error(err, "delete deploy key")
		}
		return
	}

	c.NoContent()
}
