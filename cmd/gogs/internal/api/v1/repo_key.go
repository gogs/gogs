package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func composeDeployKeysAPILink(c *apiContext) string {
	return c.BaseURL + "/repos/" + c.Repo.Owner.Name + "/" + c.Repo.Repository.Name + "/keys/"
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#list-deploy-keys
func listDeployKeys(c *apiContext) {
	keys, err := database.Handle.DeployKeys().List(c.Request().Context(), c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "list deploy keys")
		return
	}

	apiLink := composeDeployKeysAPILink(c)
	apiKeys := make([]*v1types.RepositoryDeployKey, len(keys))
	for i := range keys {
		apiKeys[i] = toDeployKey(apiLink, keys[i])
	}

	c.JSONSuccess(&apiKeys)
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#get-a-deploy-key
func getDeployKey(c *apiContext) {
	key, err := database.Handle.DeployKeys().GetByID(c.Request().Context(), c.ParamInt64("id"))
	if err != nil {
		c.NotFoundOrError(err, "get deploy key by ID")
		return
	}

	if key.RepoID != c.Repo.Repository.ID {
		c.NotFound()
		return
	}

	apiLink := composeDeployKeysAPILink(c)
	c.JSONSuccess(toDeployKey(apiLink, key))
}

func handleCheckKeyStringError(c *apiContext, err error) {
	switch {
	case errors.Is(err, database.ErrSSHDisabled):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("SSH is disabled on this site, so SSH keys cannot be added"))
	case database.IsErrKeyCheckFailed(err):
		c.Error(err, "check key content")
	case database.IsErrKeyUnableVerify(err):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Unable to verify key content"))
	default:
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Wrap(err, "Invalid key content"))
	}
}

func handleAddKeyError(c *apiContext, err error) {
	switch {
	case database.IsErrKeyAlreadyExist(err):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Key content has been used as non-deploy key"))
	case database.IsErrDeployKeyAlreadyExist(err):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Key has already been added to this repository"))
	case database.IsErrKeyNameAlreadyUsed(err), database.IsErrDeployKeyNameAlreadyUsed(err):
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

func createDeployKey(c *apiContext, sshOpts conf.SSHOpts, authorizedKeys database.AuthorizedKeysOptions, form createDeployKeyRequest) {
	content, err := database.CheckPublicKeyString(c.Logger, sshOpts, form.Key)
	if err != nil {
		handleCheckKeyStringError(c, err)
		return
	}

	key, err := database.Handle.DeployKeys().Add(c.Request().Context(), c.Logger, authorizedKeys, c.Repo.Repository.ID, form.Title, content)
	if err != nil {
		handleAddKeyError(c, err)
		return
	}

	apiLink := composeDeployKeysAPILink(c)
	c.JSON(http.StatusCreated, toDeployKey(apiLink, key))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Deploy-Keys#remove-a-deploy-key
func deleteDeploykey(c *apiContext, authorizedKeys database.AuthorizedKeysOptions) {
	key, err := database.Handle.DeployKeys().GetByID(c.Request().Context(), c.ParamInt64("id"))
	if err != nil {
		c.NotFoundOrError(err, "get deploy key by ID")
		return
	}

	if key.RepoID != c.Repo.Repository.ID {
		c.NotFound()
		return
	}

	if err := database.Handle.DeployKeys().Delete(c.Request().Context(), c.Logger, authorizedKeys, c.User, key.ID); err != nil {
		if database.IsErrKeyAccessDenied(err) {
			c.ErrorStatus(http.StatusForbidden, errors.New("You do not have access to this key"))
		} else {
			c.Error(err, "delete deploy key")
		}
		return
	}

	c.NoContent()
}
