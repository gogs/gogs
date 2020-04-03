// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-macaron/inject"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

// RegisterRoutes registers LFS routes using given router, and inherits all groups and middleware.
func RegisterRoutes(r *macaron.Router) {
	r.Group("/objects/batch", func() {
		r.Post("", verifyAcceptHeader(), serveBatch)
		r.Group("/:oid", func() {
			r.Combo("").Get(serveBatchDownload).Post(serveBatchUpload)
			r.Post("/verify", verifyAcceptHeader(), serveBatchVerify)
		})
	}, authenticate())
}

// authenticate tries to authenticate user via HTTP Basic Auth.
func authenticate() macaron.Handler {
	return func(w http.ResponseWriter, r *http.Request, injector inject.Injector) {
		username, password := auth.DecodeBasic(r.Header)
		if username == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="."`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		user, err := db.Users.Authenticate(username, password, -1)
		if err != nil && !db.IsErrUserNotExist(err) {
			w.WriteHeader(http.StatusInternalServerError)
			log.Error("Failed to authenticate user [name: %s]: %v", username, err)
			return
		}

		if err == nil && user.IsEnabledTwoFactor() {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`Users with 2FA enabled are not allowed to authenticate via username and password.`))
			return
		}

		// If username and password authentication failed, try again using username as an access token.
		if db.IsErrUserNotExist(err) {
			token, err := db.AccessTokens.GetBySHA(username)
			if err != nil {
				if db.IsErrAccessTokenNotExist(err) {
					w.Header().Set("WWW-Authenticate", `Basic realm="."`)
					w.WriteHeader(http.StatusUnauthorized)
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					log.Error("Failed to get access token [sha: %s]: %v", username, err)
				}
				return
			}
			token.Updated = time.Now()
			// TODO: update token.Updated in database

			user, err = db.Users.GetByID(token.UserID)
			if err != nil {
				// Once we found the token, we're supposed to find its related user,
				// thus any error is unexpected.
				w.WriteHeader(http.StatusInternalServerError)
				log.Error("Failed to get user: %v", err)
				return
			}
		}

		log.Trace("[LFS] Authenticated user: %s", user.Name)

		injector.Map(user)
	}
}

const contentType = "application/vnd.git-lfs+json"

// verifyAcceptHeader checks if the "Accept" header is "application/vnd.git-lfs+json".
func verifyAcceptHeader() macaron.Handler {
	return func(c *context.Context) {
		if c.Header().Get("Accept") != contentType {
			c.Status(http.StatusNotAcceptable)
			return
		}
	}
}

func serveBatch(c *context.Context) {
	var body struct {
		Operation string   `json:"operation"`
		Transfers []string `json:"transfers"`
		Objects   []struct {
			Oid  string `json:"oid"`
			Size int    `json:"size"`
		} `json:"objects"`
	}

	defer c.Req.Request.Body.Close()
	err := json.NewDecoder(c.Req.Request.Body).Decode(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}
}

func serveBatchUpload() {

}

func serveBatchDownload() {

}

func serveBatchVerify() {

}
