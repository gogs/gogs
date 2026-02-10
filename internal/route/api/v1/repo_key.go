package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func composeDeployKeysAPILink(repoPath string) string {
	return conf.Server.ExternalURL + "api/v1/repos/" + repoPath + "/keys/"
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#list-deploy-keys
func listDeployKeys(c *context.APIContext) {
	keys, err := database.ListDeployKeys(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "list deploy keys")
		return
	}

	apiLink := composeDeployKeysAPILink(c.Repo.Owner.Name + "/" + c.Repo.Repository.Name)
	apiKeys := make([]*types.RepositoryDeployKey, len(keys))
	for i := range keys {
		if err = keys[i].GetContent(); err != nil {
			c.Error(err, "get content")
			return
		}
		apiKeys[i] = toDeployKey(apiLink, keys[i])
	}

	c.JSONSuccess(&apiKeys)
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#get-a-deploy-key
func getDeployKey(c *context.APIContext) {
	key, err := database.GetDeployKeyByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get deploy key by ID")
		return
	}

	if key.RepoID != c.Repo.Repository.ID {
		c.NotFound()
		return
	}

	if err = key.GetContent(); err != nil {
		c.Error(err, "get content")
		return
	}

	apiLink := composeDeployKeysAPILink(c.Repo.Owner.Name + "/" + c.Repo.Repository.Name)
	c.JSONSuccess(toDeployKey(apiLink, key))
}

func handleCheckKeyStringError(c *context.APIContext, err error) {
	if database.IsErrKeyUnableVerify(err) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Unable to verify key content"))
	} else {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Wrap(err, "Invalid key content: %v"))
	}
}

func handleAddKeyError(c *context.APIContext, err error) {
	switch {
	case database.IsErrKeyAlreadyExist(err):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Key content has been used as non-deploy key"))
	case database.IsErrKeyNameAlreadyUsed(err):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Key title has been used"))
	default:
		c.Error(err, "add key")
	}
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#add-a-new-deploy-key
type createDeployKeyRequest struct {
	Title string `json:"title" binding:"Required"`
	Key   string `json:"key" binding:"Required"`
}

func createDeployKey(c *context.APIContext, form createDeployKeyRequest) {
	content, err := database.CheckPublicKeyString(form.Key)
	if err != nil {
		handleCheckKeyStringError(c, err)
		return
	}

	key, err := database.AddDeployKey(c.Repo.Repository.ID, form.Title, content)
	if err != nil {
		handleAddKeyError(c, err)
		return
	}

	key.Content = content
	apiLink := composeDeployKeysAPILink(c.Repo.Owner.Name + "/" + c.Repo.Repository.Name)
	c.JSON(http.StatusCreated, toDeployKey(apiLink, key))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#remove-a-deploy-key
func deleteDeploykey(c *context.APIContext) {
	key, err := database.GetDeployKeyByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get deploy key by ID")
		return
	}

	if key.RepoID != c.Repo.Repository.ID {
		c.NotFound()
		return
	}

	if err := database.DeleteDeployKey(c.User, key.ID); err != nil {
		if database.IsErrKeyAccessDenied(err) {
			c.ErrorStatus(http.StatusForbidden, errors.New("You do not have access to this key"))
		} else {
			c.Error(err, "delete deploy key")
		}
		return
	}

	c.NoContent()
}
