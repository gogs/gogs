// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	api "github.com/gogs/go-gogs-client"
	convert2 "gogs.io/gogs/internal/route/api/v1/convert"
	repo2 "gogs.io/gogs/internal/route/api/v1/repo"
	"net/http"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/setting"
)

func GetUserByParamsName(c *context.APIContext, name string) *db.User {
	user, err := db.GetUserByName(c.Params(name))
	if err != nil {
		c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
		return nil
	}
	return user
}

// GetUserByParams returns user whose name is presented in URL paramenter.
func GetUserByParams(c *context.APIContext) *db.User {
	return GetUserByParamsName(c, ":username")
}

func composePublicKeysAPILink() string {
	return setting.AppURL + "api/v1/user/keys/"
}

func listPublicKeys(c *context.APIContext, uid int64) {
	keys, err := db.ListPublicKeys(uid)
	if err != nil {
		c.ServerError("ListPublicKeys", err)
		return
	}

	apiLink := composePublicKeysAPILink()
	apiKeys := make([]*api.PublicKey, len(keys))
	for i := range keys {
		apiKeys[i] = convert2.ToPublicKey(apiLink, keys[i])
	}

	c.JSONSuccess(&apiKeys)
}

func ListMyPublicKeys(c *context.APIContext) {
	listPublicKeys(c, c.User.ID)
}

func ListPublicKeys(c *context.APIContext) {
	user := GetUserByParams(c)
	if c.Written() {
		return
	}
	listPublicKeys(c, user.ID)
}

func GetPublicKey(c *context.APIContext) {
	key, err := db.GetPublicKeyByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetPublicKeyByID", db.IsErrKeyNotExist, err)
		return
	}

	apiLink := composePublicKeysAPILink()
	c.JSONSuccess(convert2.ToPublicKey(apiLink, key))
}

// CreateUserPublicKey creates new public key to given user by ID.
func CreateUserPublicKey(c *context.APIContext, form api.CreateKeyOption, uid int64) {
	content, err := db.CheckPublicKeyString(form.Key)
	if err != nil {
		repo2.HandleCheckKeyStringError(c, err)
		return
	}

	key, err := db.AddPublicKey(uid, form.Title, content)
	if err != nil {
		repo2.HandleAddKeyError(c, err)
		return
	}
	apiLink := composePublicKeysAPILink()
	c.JSON(http.StatusCreated, convert2.ToPublicKey(apiLink, key))
}

func CreatePublicKey(c *context.APIContext, form api.CreateKeyOption) {
	CreateUserPublicKey(c, form, c.User.ID)
}

func DeletePublicKey(c *context.APIContext) {
	if err := db.DeletePublicKey(c.User, c.ParamsInt64(":id")); err != nil {
		if db.IsErrKeyAccessDenied(err) {
			c.Error(http.StatusForbidden, "", "you do not have access to this key")
		} else {
			c.Error(http.StatusInternalServerError, "DeletePublicKey", err)
		}
		return
	}

	c.NoContent()
}
