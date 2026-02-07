package user

import (
	gocontext "context"
	"net/http"

	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

// TokenResponse represents an access token in API responses.
type TokenResponse struct {
	TokenName     string `json:"name"`
	TokenHashSha1 string `json:"sha1"`
}

// CreateTokenRequest represents the request body for creating an access token.
type CreateTokenRequest struct {
	TokenName string `json:"name" binding:"Required"`
}

// AccessTokensHandler is the handler for users access tokens API endpoints.
type AccessTokensHandler struct {
	store AccessTokensStore
}

// NewAccessTokensHandler returns a new AccessTokensHandler for users access
// tokens API endpoints.
func NewAccessTokensHandler(s AccessTokensStore) *AccessTokensHandler {
	return &AccessTokensHandler{
		store: s,
	}
}

func (h *AccessTokensHandler) List() macaron.Handler {
	return func(c *context.APIContext) {
		tokens, err := h.store.ListAccessTokens(c.Req.Context(), c.User.ID)
		if err != nil {
			c.Error(err, "list access tokens")
			return
		}

		apiTokens := make([]*TokenResponse, len(tokens))
		for i := range tokens {
			apiTokens[i] = &TokenResponse{
				TokenName:     tokens[i].Name,
				TokenHashSha1: tokens[i].Sha1,
			}
		}
		c.JSONSuccess(&apiTokens)
	}
}

func (h *AccessTokensHandler) Create() macaron.Handler {
	return func(c *context.APIContext, form CreateTokenRequest) {
		t, err := h.store.CreateAccessToken(c.Req.Context(), c.User.ID, form.TokenName)
		if err != nil {
			if database.IsErrAccessTokenAlreadyExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "new access token")
			}
			return
		}
		c.JSON(http.StatusCreated, &TokenResponse{
			TokenName:     t.Name,
			TokenHashSha1: t.Sha1,
		})
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
}

type accessTokensStore struct{}

// NewAccessTokensStore returns a new AccessTokensStore using the global
// database handle.
func NewAccessTokensStore() AccessTokensStore {
	return &accessTokensStore{}
}

func (*accessTokensStore) CreateAccessToken(ctx gocontext.Context, userID int64, name string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().Create(ctx, userID, name)
}

func (*accessTokensStore) ListAccessTokens(ctx gocontext.Context, userID int64) ([]*database.AccessToken, error) {
	return database.Handle.AccessTokens().List(ctx, userID)
}
