// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	gocontext "context"
	"net/http"

	api "github.com/gogs/go-gogs-client"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

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

		apiTokens := make([]*api.AccessToken, len(tokens))
		for i := range tokens {
			apiTokens[i] = &api.AccessToken{Name: tokens[i].Name, Sha1: tokens[i].Sha1}
		}
		c.JSONSuccess(&apiTokens)
	}
}

func (h *AccessTokensHandler) Create() macaron.Handler {
	return func(c *context.APIContext, form api.CreateAccessTokenOption) {
		t, err := h.store.CreateAccessToken(c.Req.Context(), c.User.ID, form.Name)
		if err != nil {
			if database.IsErrAccessTokenAlreadyExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "new access token")
			}
			return
		}
		c.JSON(http.StatusCreated, &api.AccessToken{Name: t.Name, Sha1: t.Sha1})
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

// Note: the possibility, though remote, of SHA1 collissions could be made far less likely via providing a user as well.
func (*accessTokensStore) GetBySHA1(ctx gocontext.Context, Sha1 string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().GetBySHA1(ctx, Sha1)
}

func (*accessTokensStore) DeleteAccessToken(ctx gocontext.Context, userID int64, tokenID int64) error {
	return database.Handle.AccessTokens().DeleteByID(ctx, userID, tokenID)
}
