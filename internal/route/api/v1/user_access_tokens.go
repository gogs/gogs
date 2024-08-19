package v1

import (
	gocontext "context"
	"net/http"

	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
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

func (h *accessTokensHandler) List() macaron.Handler {
	return func(c *context.APIContext) {
		tokens, err := h.store.ListAccessTokens(c.Req.Context(), c.User.ID)
		if err != nil {
			c.Error(err, "list access tokens")
			return
		}

		apiTokens := make([]*types.UserAccessToken, len(tokens))
		for i := range tokens {
			apiTokens[i] = &types.UserAccessToken{
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

func (h *accessTokensHandler) Create() macaron.Handler {
	return func(c *context.APIContext, form createAccessTokenRequest) {
		t, err := h.store.CreateAccessToken(c.Req.Context(), c.User.ID, form.Name)
		if err != nil {
			if database.IsErrAccessTokenAlreadyExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "new access token")
			}
			return
		}
		c.JSON(http.StatusCreated, &types.UserAccessToken{
			Name: t.Name,
			Sha1: t.Sha1,
		})
	}
}

// Deletes the provided token identified by SHA1
// This prevents anyone from deleting everything in list(), as the identifiers in list() are not the actual SHA1.
func (h *AccessTokensHandler) Delete() macaron.Handler {
	//TODO make the latter arg api.DeleteAccessTokenOption
	return func(c *context.APIContext, form api.DeleteAccessTokenOption) {
		// We need the ID of the token to delete it.
		existing, err := h.store.GetBySHA1(c.Req.Context(), form.Sha1);
		if err != nil {
			if database.IsErrAccessTokenNotExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "list tokens in delete prep")
			}
			return
		}

		// We need the User ID and the Token ID to delete it.
		err = h.store.DeleteAccessToken(c.Req.Context(), c.User.ID, existing.ID)
		if err != nil {
			// Always possible that we race, TOCTOU
			if database.IsErrAccessTokenNotExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "delete access token")
			}
			return
		}
		c.JSON(http.StatusOK, &api.AccessToken{Name: existing.Name})
	}
}

// AccessTokensStore is the data layer carrier for user access tokens API
// endpoints. This interface is meant to abstract away and limit the exposure of
// the underlying data layer to the handler through a thin-wrapper.
type AccessTokensStore interface {
	// CreateAccessToken creates a new access token and persist to database. It
	// returns database.ErrAccessTokenAlreadyExist when an access token with same
	// name already exists for the user.
	CreateAccessToken(ctx gocontext.Context, userID int64, name string) (*database.AccessToken, error)
	// ListAccessTokens returns all access tokens belongs to given user.
	ListAccessTokens(ctx gocontext.Context, userID int64) ([]*database.AccessToken, error)
	// Get a token by SHA1 (used to find a tok to delete it)
	GetBySHA1(ctx gocontext.Context, Sha1 string) (*database.AccessToken, error)
	// Delete a given token
	DeleteAccessToken(ctx gocontext.Context, userID int64, tokenID int64) error
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

// Note: the possibility, though remote, of SHA1 collissions could be made far less likely via providing a user as well.
func (*accessTokensStore) GetBySHA1(ctx gocontext.Context, Sha1 string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().GetBySHA1(ctx, Sha1)
}

func (*accessTokensStore) DeleteAccessToken(ctx gocontext.Context, userID int64, tokenID int64) error {
	return database.Handle.AccessTokens().DeleteByID(ctx, userID, tokenID)
}
