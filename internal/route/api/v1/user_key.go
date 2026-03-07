package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func getUserByParamsName(c *context.APIContext, name string) *database.User {
	user, err := database.Handle.Users().GetByUsername(c.Req.Context(), c.Params(name))
	if err != nil {
		c.NotFoundOrError(err, "get user by name")
		return nil
	}
	return user
}

func getUserByParams(c *context.APIContext) *database.User {
	return getUserByParamsName(c, ":username")
}

func composePublicKeysAPILink() string {
	return conf.Server.ExternalURL + "api/v1/user/keys/"
}

func listPublicKeysOfUser(c *context.APIContext, uid int64) {
	keys, err := database.ListPublicKeys(uid)
	if err != nil {
		c.Error(err, "list public keys")
		return
	}

	apiLink := composePublicKeysAPILink()
	apiKeys := make([]*types.UserPublicKey, len(keys))
	for i := range keys {
		apiKeys[i] = toUserPublicKey(apiLink, keys[i])
	}

	c.JSONSuccess(&apiKeys)
}

func listMyPublicKeys(c *context.APIContext) {
	listPublicKeysOfUser(c, c.User.ID)
}

func listPublicKeys(c *context.APIContext) {
	user := getUserByParams(c)
	if c.Written() {
		return
	}
	listPublicKeysOfUser(c, user.ID)
}

func getPublicKey(c *context.APIContext) {
	key, err := database.GetPublicKeyByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get public key by ID")
		return
	}

	apiLink := composePublicKeysAPILink()
	c.JSONSuccess(toUserPublicKey(apiLink, key))
}

type createPublicKeyRequest struct {
	Title string `json:"title" binding:"Required"`
	Key   string `json:"key" binding:"Required"`
}

func createUserPublicKey(c *context.APIContext, form createPublicKeyRequest, uid int64) {
	content, err := database.CheckPublicKeyString(form.Key)
	if err != nil {
		handleCheckKeyStringError(c, err)
		return
	}

	key, err := database.AddPublicKey(uid, form.Title, content)
	if err != nil {
		handleAddKeyError(c, err)
		return
	}
	apiLink := composePublicKeysAPILink()
	c.JSON(http.StatusCreated, toUserPublicKey(apiLink, key))
}

func createPublicKey(c *context.APIContext, form createPublicKeyRequest) {
	createUserPublicKey(c, form, c.User.ID)
}

func deletePublicKey(c *context.APIContext) {
	if err := database.DeletePublicKey(c.User, c.ParamsInt64(":id")); err != nil {
		if database.IsErrKeyAccessDenied(err) {
			c.ErrorStatus(http.StatusForbidden, errors.New("You do not have access to this key."))
		} else {
			c.Error(err, "delete public key")
		}
		return
	}

	c.NoContent()
}
