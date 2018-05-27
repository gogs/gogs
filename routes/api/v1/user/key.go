// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/routes/api/v1/convert"
	"github.com/gogs/gogs/routes/api/v1/repo"
)

func GetUserByParamsName(c *context.APIContext, name string) *models.User {
	user, err := models.GetUserByName(c.Params(name))
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetUserByName", err)
		}
		return nil
	}
	return user
}

// GetUserByParams returns user whose name is presented in URL paramenter.
func GetUserByParams(c *context.APIContext) *models.User {
	return GetUserByParamsName(c, ":username")
}

func composePublicKeysAPILink() string {
	return setting.AppURL + "api/v1/user/keys/"
}

func listPublicKeys(c *context.APIContext, uid int64) {
	keys, err := models.ListPublicKeys(uid)
	if err != nil {
		c.Error(500, "ListPublicKeys", err)
		return
	}

	apiLink := composePublicKeysAPILink()
	apiKeys := make([]*api.PublicKey, len(keys))
	for i := range keys {
		apiKeys[i] = convert.ToPublicKey(apiLink, keys[i])
	}

	c.JSON(200, &apiKeys)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Public-Keys#list-your-public-keys
func ListMyPublicKeys(c *context.APIContext) {
	listPublicKeys(c, c.User.ID)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Public-Keys#list-public-keys-for-a-user
func ListPublicKeys(c *context.APIContext) {
	user := GetUserByParams(c)
	if c.Written() {
		return
	}
	listPublicKeys(c, user.ID)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Public-Keys#get-a-single-public-key
func GetPublicKey(c *context.APIContext) {
	key, err := models.GetPublicKeyByID(c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrKeyNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetPublicKeyByID", err)
		}
		return
	}

	apiLink := composePublicKeysAPILink()
	c.JSON(200, convert.ToPublicKey(apiLink, key))
}

// CreateUserPublicKey creates new public key to given user by ID.
func CreateUserPublicKey(c *context.APIContext, form api.CreateKeyOption, uid int64) {
	content, err := models.CheckPublicKeyString(form.Key)
	if err != nil {
		repo.HandleCheckKeyStringError(c, err)
		return
	}

	key, err := models.AddPublicKey(uid, form.Title, content)
	if err != nil {
		repo.HandleAddKeyError(c, err)
		return
	}
	apiLink := composePublicKeysAPILink()
	c.JSON(201, convert.ToPublicKey(apiLink, key))
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Public-Keys#create-a-public-key
func CreatePublicKey(c *context.APIContext, form api.CreateKeyOption) {
	CreateUserPublicKey(c, form, c.User.ID)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Public-Keys#delete-a-public-key
func DeletePublicKey(c *context.APIContext) {
	if err := models.DeletePublicKey(c.User, c.ParamsInt64(":id")); err != nil {
		if models.IsErrKeyAccessDenied(err) {
			c.Error(403, "", "You do not have access to this key")
		} else {
			c.Error(500, "DeletePublicKey", err)
		}
		return
	}

	c.Status(204)
}
