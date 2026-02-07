package user

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/repo"
)

// PkResp holds public key response data.
type PkResp struct {
	IdVal     int64     `json:"id"`
	KeyTxt    string    `json:"key"`
	UrlStr    string    `json:"url"`
	TitleTxt  string    `json:"title"`
	CreatedTm time.Time `json:"created_at"`
}

// PkReq holds public key request data.
type PkReq struct {
	KeyTxt   string `json:"key" binding:"Required"`
	TitleTxt string `json:"title" binding:"Required"`
}

func buildPkResp(apiLink string, k *database.PublicKey) *PkResp {
	r := PkResp{IdVal: k.ID}
	r.KeyTxt = k.Content
	r.TitleTxt = k.Name
	r.CreatedTm = k.Created
	r.UrlStr = fmt.Sprintf("%s%d", apiLink, r.IdVal)
	return &r
}

func GetUserByParamsName(c *context.APIContext, name string) *database.User {
	user, err := database.Handle.Users().GetByUsername(c.Req.Context(), c.Params(name))
	if err != nil {
		c.NotFoundOrError(err, "get user by name")
		return nil
	}
	return user
}

func GetUserByParams(c *context.APIContext) *database.User {
	return GetUserByParamsName(c, ":username")
}

func composePublicKeysAPILink() string {
	return conf.Server.ExternalURL + "api/v1/user/keys/"
}

func listPublicKeys(c *context.APIContext, uid int64) {
	keys, err := database.ListPublicKeys(uid)
	if err != nil {
		c.Error(err, "list public keys")
		return
	}

	apiLink := composePublicKeysAPILink()
	resps := make([]*PkResp, len(keys))
	for i := range keys {
		resps[i] = buildPkResp(apiLink, keys[i])
	}

	c.JSONSuccess(&resps)
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
	key, err := database.GetPublicKeyByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get public key by ID")
		return
	}

	apiLink := composePublicKeysAPILink()
	c.JSONSuccess(buildPkResp(apiLink, key))
}

func CreateUserPublicKey(c *context.APIContext, form PkReq, uid int64) {
	content, err := database.CheckPublicKeyString(form.KeyTxt)
	if err != nil {
		repo.HandleCheckKeyStringError(c, err)
		return
	}

	key, err := database.AddPublicKey(uid, form.TitleTxt, content)
	if err != nil {
		repo.HandleAddKeyError(c, err)
		return
	}
	apiLink := composePublicKeysAPILink()
	c.JSON(http.StatusCreated, buildPkResp(apiLink, key))
}

func CreatePublicKey(c *context.APIContext, form PkReq) {
	CreateUserPublicKey(c, form, c.User.ID)
}

func DeletePublicKey(c *context.APIContext) {
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
