package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func getUserByParamsName(c *apiContext, name string) *database.User {
	user, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param(name))
	if err != nil {
		c.NotFoundOrError(err, "get user by name")
		return nil
	}
	return user
}

func getUserByParams(c *apiContext) *database.User {
	return getUserByParamsName(c, "username")
}

func composePublicKeysAPILink() string {
	return conf.Server.ExternalURL + "api/v1/user/keys/"
}

func listPublicKeysOfUser(c *apiContext, uid int64) {
	keys, err := database.Handle.PublicKeys().List(c.Request().Context(), uid)
	if err != nil {
		c.Error(err, "list public keys")
		return
	}

	apiLink := composePublicKeysAPILink()
	apiKeys := make([]*v1types.UserPublicKey, len(keys))
	for i := range keys {
		apiKeys[i] = toUserPublicKey(apiLink, keys[i])
	}

	c.JSONSuccess(&apiKeys)
}

func listMyPublicKeys(c *apiContext) {
	listPublicKeysOfUser(c, c.User.ID)
}

func listPublicKeys(c *apiContext) {
	user := getUserByParams(c)
	if c.Written() {
		return
	}
	listPublicKeysOfUser(c, user.ID)
}

func getPublicKey(c *apiContext) {
	key, err := database.Handle.PublicKeys().GetByID(c.Request().Context(), c.ParamInt64("id"))
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

func createUserPublicKey(c *apiContext, form createPublicKeyRequest, uid int64) {
	content, err := database.CheckPublicKeyString(c.Logger, form.Key)
	if err != nil {
		handleCheckKeyStringError(c, err)
		return
	}

	key, err := database.Handle.PublicKeys().Add(c.Request().Context(), c.Logger, uid, form.Title, content)
	if err != nil {
		handleAddKeyError(c, err)
		return
	}
	apiLink := composePublicKeysAPILink()
	c.JSON(http.StatusCreated, toUserPublicKey(apiLink, key))
}

func createPublicKey(c *apiContext, form createPublicKeyRequest) {
	createUserPublicKey(c, form, c.User.ID)
}

func deletePublicKey(c *apiContext) {
	if err := database.Handle.PublicKeys().Delete(c.Request().Context(), c.Logger, c.User, c.ParamInt64("id")); err != nil {
		if database.IsErrKeyAccessDenied(err) {
			c.ErrorStatus(http.StatusForbidden, errors.New("You do not have access to this key."))
		} else {
			c.Error(err, "delete public key")
		}
		return
	}

	c.NoContent()
}
