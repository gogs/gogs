package v1

import (
	gocontext "context"
	"net/http"
	"slices"

	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

// accessTokensHandler is the handler for users access tokens API endpoints.
type accessTokensHandler struct {
	store AccessTokensStore
}

// newAccessTokensHandler returns a new accessTokensHandler for users access
// tokens API endpoints.
func newAccessTokensHandler(s AccessTokensStore) *accessTokensHandler {
	return &accessTokensHandler{
		store: s,
	}
}

func (h *accessTokensHandler) List() flamego.Handler {
	return func(c *apiContext) {
		tokens, err := h.store.ListAccessTokens(c.Request().Context(), c.User.ID)
		if err != nil {
			c.Error(err, "list access tokens")
			return
		}

		apiTokens := make([]*types.UserAccessToken, len(tokens))
		for i := range tokens {
			apiTokens[i] = &types.UserAccessToken{
				ID:   tokens[i].ID,
				Name: tokens[i].Name,
				Sha1: tokens[i].Sha1,
			}
		}
		c.JSONSuccess(&apiTokens)
	}
}

type createAccessTokenRequest struct {
	Name string `json:"name" binding:"Required"`
}

func (h *accessTokensHandler) Create() flamego.Handler {
	return func(c *apiContext, form createAccessTokenRequest) {
		t, err := h.store.CreateAccessToken(c.Request().Context(), c.User.ID, form.Name)
		if err != nil {
			if database.IsErrAccessTokenAlreadyExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "new access token")
			}
			return
		}
		c.JSON(http.StatusCreated, &types.UserAccessToken{
			ID:   t.ID,
			Name: t.Name,
			Sha1: t.Sha1,
		})
	}
}

func (h *accessTokensHandler) Delete() flamego.Handler {
	return func(c *apiContext) {
		ctx := c.Request().Context()
		id := c.ParamInt64("id")
		tokens, err := h.store.ListAccessTokens(ctx, c.User.ID)
		if err != nil {
			c.Error(err, "list access tokens")
			return
		}
		if !slices.ContainsFunc(tokens, func(t *database.AccessToken) bool { return t.ID == id }) {
			c.NotFound()
			return
		}

		if err = h.store.DeleteAccessTokenByID(ctx, c.User.ID, id); err != nil {
			c.Error(err, "delete access token")
			return
		}
		c.NoContent()
	}
}

// AccessTokensStore is the data layer carrier for user access tokens API
// endpoints. This interface is meant to abstract away and limit the exposure of
// the underlying data layer to the handler through a thin-wrapper.
type AccessTokensStore interface {
	// CreateAccessToken creates a new access token and persists to database. It
	// returns database.ErrAccessTokenAlreadyExist when an access token with same
	// name already exists for the user.
	CreateAccessToken(ctx gocontext.Context, userID int64, name string) (*database.AccessToken, error)
	// ListAccessTokens returns all access tokens that belong to given user.
	ListAccessTokens(ctx gocontext.Context, userID int64) ([]*database.AccessToken, error)
	// DeleteAccessTokenByID deletes the access token of the given ID that belongs
	// to the given user.
	DeleteAccessTokenByID(ctx gocontext.Context, userID, id int64) error
}

type accessTokensStore struct{}

// newAccessTokensStore returns a new AccessTokensStore using the global
// database handle.
func newAccessTokensStore() AccessTokensStore {
	return &accessTokensStore{}
}

func (*accessTokensStore) CreateAccessToken(ctx gocontext.Context, userID int64, name string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().Create(ctx, userID, name)
}

func (*accessTokensStore) ListAccessTokens(ctx gocontext.Context, userID int64) ([]*database.AccessToken, error) {
	return database.Handle.AccessTokens().List(ctx, userID)
}

func (*accessTokensStore) DeleteAccessTokenByID(ctx gocontext.Context, userID, id int64) error {
	return database.Handle.AccessTokens().DeleteByID(ctx, userID, id)
}
