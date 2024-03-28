// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"net/http"
	"strings"

	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/authutil"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsutil"
)

// RegisterRoutes registers LFS routes using given router, and inherits all
// groups and middleware.
func RegisterRoutes(r *macaron.Router) {
	verifyAccept := verifyHeader("Accept", contentType, http.StatusNotAcceptable)
	verifyContentTypeJSON := verifyHeader("Content-Type", contentType, http.StatusBadRequest)
	verifyContentTypeStream := verifyHeader("Content-Type", "application/octet-stream", http.StatusBadRequest)

	store := NewStore()
	r.Group("", func() {
		r.Post("/objects/batch", authorize(store, database.AccessModeRead), verifyAccept, verifyContentTypeJSON, serveBatch(store))
		r.Group("/objects/basic", func() {
			basic := &basicHandler{
				store:          store,
				defaultStorage: lfsutil.Storage(conf.LFS.Storage),
				storagers: map[lfsutil.Storage]lfsutil.Storager{
					lfsutil.StorageLocal: &lfsutil.LocalStorage{Root: conf.LFS.ObjectsPath},
				},
			}
			r.Combo("/:oid", verifyOID()).
				Get(authorize(store, database.AccessModeRead), basic.serveDownload).
				Put(authorize(store, database.AccessModeWrite), verifyContentTypeStream, basic.serveUpload)
			r.Post("/verify", authorize(store, database.AccessModeWrite), verifyAccept, verifyContentTypeJSON, basic.serveVerify)
		})
	}, authenticate(store))
}

// authenticate tries to authenticate user via HTTP Basic Auth. It first tries to authenticate
// as plain username and password, then use username as access token if previous step failed.
func authenticate(store Store) macaron.Handler {
	askCredentials := func(w http.ResponseWriter) {
		w.Header().Set("Lfs-Authenticate", `Basic realm="Git LFS"`)
		responseJSON(w, http.StatusUnauthorized, responseError{
			Message: "Credentials needed",
		})
	}

	return func(c *macaron.Context) {
		username, password := authutil.DecodeBasic(c.Req.Header)
		if username == "" {
			askCredentials(c.Resp)
			return
		}

		user, err := database.Users.Authenticate(c.Req.Context(), username, password, -1)
		if err != nil && !auth.IsErrBadCredentials(err) {
			internalServerError(c.Resp)
			log.Error("Failed to authenticate user [name: %s]: %v", username, err)
			return
		}

		if err == nil && store.IsTwoFactorEnabled(c.Req.Context(), user.ID) {
			c.Error(http.StatusBadRequest, "Users with 2FA enabled are not allowed to authenticate via username and password.")
			return
		}

		// If username and password combination failed, try again using either username
		// or password as the token.
		if auth.IsErrBadCredentials(err) {
			user, err = context.AuthenticateByToken(store, c.Req.Context(), username)
			if err != nil && !database.IsErrAccessTokenNotExist(err) {
				internalServerError(c.Resp)
				log.Error("Failed to authenticate by access token via username: %v", err)
				return
			} else if database.IsErrAccessTokenNotExist(err) {
				// Try again using the password field as the token.
				user, err = context.AuthenticateByToken(store, c.Req.Context(), password)
				if err != nil {
					if database.IsErrAccessTokenNotExist(err) {
						askCredentials(c.Resp)
					} else {
						c.Status(http.StatusInternalServerError)
						log.Error("Failed to authenticate by access token via password: %v", err)
					}
					return
				}
			}
		}

		log.Trace("[LFS] Authenticated user: %s", user.Name)

		c.Map(user)
	}
}

// authorize tries to authorize the user to the context repository with given access mode.
func authorize(store Store, mode database.AccessMode) macaron.Handler {
	return func(c *macaron.Context, actor *database.User) {
		username := c.Params(":username")
		reponame := strings.TrimSuffix(c.Params(":reponame"), ".git")

		owner, err := database.Users.GetByUsername(c.Req.Context(), username)
		if err != nil {
			if database.IsErrUserNotExist(err) {
				c.Status(http.StatusNotFound)
			} else {
				internalServerError(c.Resp)
				log.Error("Failed to get user [name: %s]: %v", username, err)
			}
			return
		}

		repo, err := store.GetRepositoryByName(c.Req.Context(), owner.ID, reponame)
		if err != nil {
			if database.IsErrRepoNotExist(err) {
				c.Status(http.StatusNotFound)
			} else {
				internalServerError(c.Resp)
				log.Error("Failed to get repository [owner_id: %d, name: %s]: %v", owner.ID, reponame, err)
			}
			return
		}

		if !store.AuthorizeRepositoryAccess(c.Req.Context(), actor.ID, repo.ID, mode,
			database.AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			},
		) {
			c.Status(http.StatusNotFound)
			return
		}

		log.Trace("[LFS] Authorized user %q to %q", actor.Name, username+"/"+reponame)

		c.Map(owner) // NOTE: Override actor
		c.Map(repo)
	}
}

// verifyHeader checks if the HTTP header contains given value.
// When not, response given "failCode" as status code.
func verifyHeader(key, value string, failCode int) macaron.Handler {
	return func(c *macaron.Context) {
		vals := c.Req.Header.Values(key)
		for _, val := range vals {
			if strings.Contains(val, value) {
				return
			}
		}

		log.Trace("[LFS] HTTP header %q does not contain value %q", key, value)
		c.Status(failCode)
	}
}

// verifyOID checks if the ":oid" URL parameter is valid.
func verifyOID() macaron.Handler {
	return func(c *macaron.Context) {
		oid := lfsutil.OID(c.Params(":oid"))
		if !lfsutil.ValidOID(oid) {
			responseJSON(c.Resp, http.StatusBadRequest, responseError{
				Message: "Invalid oid",
			})
			return
		}

		c.Map(oid)
	}
}

func internalServerError(w http.ResponseWriter) {
	responseJSON(w, http.StatusInternalServerError, responseError{
		Message: "Internal server error",
	})
}
