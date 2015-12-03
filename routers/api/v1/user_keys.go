// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"github.com/Unknwon/com"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

func ToApiPublicKey(apiLink string, key *models.PublicKey) *api.PublicKey {
	return &api.PublicKey{
		ID:      key.ID,
		Key:     key.Content,
		URL:     apiLink + com.ToStr(key.ID),
		Title:   key.Name,
		Created: key.Created,
	}
}

func composePublicKeysAPILink() string {
	return setting.AppUrl + "api/v1/user/keys/"
}

func listUserPublicKeys(ctx *middleware.Context, uid int64) {
	keys, err := models.ListPublicKeys(uid)
	if err != nil {
		ctx.APIError(500, "ListPublicKeys", err)
		return
	}

	apiLink := composePublicKeysAPILink()
	apiKeys := make([]*api.PublicKey, len(keys))
	for i := range keys {
		apiKeys[i] = ToApiPublicKey(apiLink, keys[i])
	}

	ctx.JSON(200, &apiKeys)
}

// https://github.com/gogits/go-gogs-client/wiki/Users-Public-Keys#list-public-keys-for-a-user
func ListUserPublicKeys(ctx *middleware.Context) {
	user, err := models.GetUserByName(ctx.Params(":username"))
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Error(404)
		} else {
			ctx.APIError(500, "GetUserByName", err)
		}
		return
	}
	listUserPublicKeys(ctx, user.Id)
}

// https://github.com/gogits/go-gogs-client/wiki/Users-Public-Keys#list-your-public-keys
func ListMyPublicKeys(ctx *middleware.Context) {
	listUserPublicKeys(ctx, ctx.User.Id)
}

// https://github.com/gogits/go-gogs-client/wiki/Users-Public-Keys#get-a-single-public-key
func GetUserPublicKey(ctx *middleware.Context) {
	key, err := models.GetPublicKeyByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrKeyNotExist(err) {
			ctx.Error(404)
		} else {
			ctx.Handle(500, "GetPublicKeyByID", err)
		}
		return
	}

	apiLink := composePublicKeysAPILink()
	ctx.JSON(200, ToApiPublicKey(apiLink, key))
}

// https://github.com/gogits/go-gogs-client/wiki/Users-Public-Keys#create-a-public-key
func CreateUserPublicKey(ctx *middleware.Context, form api.CreateKeyOption) {
	content, err := models.CheckPublicKeyString(form.Key)
	if err != nil {
		handleCheckKeyStringError(ctx, err)
		return
	}

	key, err := models.AddPublicKey(ctx.User.Id, form.Title, content)
	if err != nil {
		handleAddKeyError(ctx, err)
		return
	}
	apiLink := composePublicKeysAPILink()
	ctx.JSON(201, ToApiPublicKey(apiLink, key))
}

// https://github.com/gogits/go-gogs-client/wiki/Users-Public-Keys#delete-a-public-key
func DeleteUserPublicKey(ctx *middleware.Context) {
	if err := models.DeletePublicKey(ctx.User, ctx.ParamsInt64(":id")); err != nil {
		if models.IsErrKeyAccessDenied(err) {
			ctx.APIError(403, "", "You do not have access to this key")
		} else {
			ctx.APIError(500, "DeletePublicKey", err)
		}
		return
	}

	ctx.Status(204)
}
