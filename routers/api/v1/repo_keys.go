// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"fmt"

	"github.com/Unknwon/com"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

func ToApiDeployKey(apiLink string, key *models.DeployKey) *api.DeployKey {
	return &api.DeployKey{
		ID:       key.ID,
		Key:      key.Content,
		URL:      apiLink + com.ToStr(key.ID),
		Title:    key.Name,
		Created:  key.Created,
		ReadOnly: true, // All deploy keys are read-only.
	}
}

func composeDeployKeysAPILink(repoPath string) string {
	return setting.AppUrl + "api/v1/repos/" + repoPath + "/keys/"
}

// https://github.com/gogits/go-gogs-client/wiki/Repositories---Deploy-Keys#list-deploy-keys
func ListRepoDeployKeys(ctx *middleware.Context) {
	keys, err := models.ListDeployKeys(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "ListDeployKeys", err)
		return
	}

	apiLink := composeDeployKeysAPILink(ctx.Repo.Owner.Name + "/" + ctx.Repo.Repository.Name)
	apiKeys := make([]*api.DeployKey, len(keys))
	for i := range keys {
		if err = keys[i].GetContent(); err != nil {
			ctx.APIError(500, "GetContent", err)
			return
		}
		apiKeys[i] = ToApiDeployKey(apiLink, keys[i])
	}

	ctx.JSON(200, &apiKeys)
}

// https://github.com/gogits/go-gogs-client/wiki/Repositories---Deploy-Keys#get-a-deploy-key
func GetRepoDeployKey(ctx *middleware.Context) {
	key, err := models.GetDeployKeyByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrDeployKeyNotExist(err) {
			ctx.Error(404)
		} else {
			ctx.Handle(500, "GetDeployKeyByID", err)
		}
		return
	}

	if err = key.GetContent(); err != nil {
		ctx.APIError(500, "GetContent", err)
		return
	}

	apiLink := composeDeployKeysAPILink(ctx.Repo.Owner.Name + "/" + ctx.Repo.Repository.Name)
	ctx.JSON(200, ToApiDeployKey(apiLink, key))
}

// https://github.com/gogits/go-gogs-client/wiki/Repositories---Deploy-Keys#add-a-new-deploy-key
func CreateRepoDeployKey(ctx *middleware.Context, form api.CreateDeployKeyOption) {
	content, err := models.CheckPublicKeyString(form.Key)
	if err != nil {
		if models.IsErrKeyUnableVerify(err) {
			ctx.APIError(422, "", "Unable to verify key content")
		} else {
			ctx.APIError(422, "", fmt.Errorf("Invalid key content: %v", err))
		}
		return
	}

	key, err := models.AddDeployKey(ctx.Repo.Repository.ID, form.Title, content)
	if err != nil {
		ctx.Data["HasError"] = true
		switch {
		case models.IsErrKeyAlreadyExist(err):
			ctx.APIError(422, "", "Key content has been used as non-deploy key")
		case models.IsErrKeyNameAlreadyUsed(err):
			ctx.APIError(422, "", "Key title has been used")
		default:
			ctx.APIError(500, "AddDeployKey", err)
		}
		return
	}

	key.Content = content
	apiLink := composeDeployKeysAPILink(ctx.Repo.Owner.Name + "/" + ctx.Repo.Repository.Name)
	ctx.JSON(201, ToApiDeployKey(apiLink, key))
}

// https://github.com/gogits/go-gogs-client/wiki/Repositories---Deploy-Keys#remove-a-deploy-key
func DeleteRepoDeploykey(ctx *middleware.Context) {
	if err := models.DeleteDeployKey(ctx.ParamsInt64(":id")); err != nil {
		ctx.APIError(500, "DeleteDeployKey", err)
		return
	}

	ctx.Status(204)
}
