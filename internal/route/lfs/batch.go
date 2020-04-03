// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

// RegisterRoutes registers LFS routes using given router, and inherits all groups and middleware.
func RegisterRoutes(r *macaron.Router) {
	r.Group("/objects/batch", func() {
		r.Post("", authorize(db.AccessModeRead), verifyAcceptHeader(), serveBatch)
		r.Group("/:oid", func() {
			r.Combo("").
				Get(authorize(db.AccessModeRead), serveBatchDownload).
				Post(authorize(db.AccessModeWrite), serveBatchUpload)
			r.Post("/verify", authorize(db.AccessModeWrite), verifyAcceptHeader(), serveBatchVerify)
		})
	}, authenticate())
}

// authenticate tries to authenticate user via HTTP Basic Auth.
func authenticate() macaron.Handler {
	return func(c *context.Context) {
		username, password := auth.DecodeBasic(c.Req.Header)
		if username == "" {
			c.Header().Set("WWW-Authenticate", `Basic realm="."`)
			c.Status(http.StatusUnauthorized)
			return
		}

		user, err := db.Users.Authenticate(username, password, -1)
		if err != nil && !db.IsErrUserNotExist(err) {
			c.Status(http.StatusInternalServerError)
			log.Error("Failed to authenticate user [name: %s]: %v", username, err)
			return
		}

		if err == nil && user.IsEnabledTwoFactor() {
			c.PlainText(http.StatusBadRequest, `Users with 2FA enabled are not allowed to authenticate via username and password.`)
			return
		}

		// If username and password authentication failed, try again using username as an access token.
		if db.IsErrUserNotExist(err) {
			token, err := db.AccessTokens.GetBySHA(username)
			if err != nil {
				if db.IsErrAccessTokenNotExist(err) {
					c.Header().Set("WWW-Authenticate", `Basic realm="."`)
					c.Status(http.StatusUnauthorized)
				} else {
					c.Status(http.StatusInternalServerError)
					log.Error("Failed to get access token [sha: %s]: %v", username, err)
				}
				return
			}
			token.Updated = time.Now()
			if err = db.AccessTokens.Save(token); err != nil {
				log.Error("Failed to update access token: %v", err)
			}

			user, err = db.Users.GetByID(token.UserID)
			if err != nil {
				// Once we found the token, we're supposed to find its related user,
				// thus any error is unexpected.
				c.Status(http.StatusInternalServerError)
				log.Error("Failed to get user: %v", err)
				return
			}
		}

		log.Trace("[LFS] Authenticated user: %s", user.Name)

		c.Map(user)
	}
}

// authorize tries to authorize the user to the context repository with given access mode.
func authorize(mode db.AccessMode) macaron.Handler {
	return func(c *context.Context, user *db.User) {
		username := c.Params(":username")
		reponame := strings.TrimSuffix(c.Params(":reponame"), ".git")

		owner, err := db.Users.GetByUsername(username)
		if err != nil {
			if db.IsErrUserNotExist(err) {
				c.Status(http.StatusNotFound)
			} else {
				c.Status(http.StatusInternalServerError)
				log.Error("Failed to get user [name: %s]: %v", username, err)
			}
			return
		}

		repo, err := db.Repos.GetByName(owner.ID, reponame)
		if err != nil {
			if db.IsErrRepoNotExist(err) {
				c.Status(http.StatusNotFound)
			} else {
				c.Status(http.StatusInternalServerError)
				log.Error("Failed to get repository [owner_id: %d, name: %s]: %v", owner.ID, reponame, err)
			}
			return
		}

		if !db.Perms.Authorize(user.ID, repo, mode) {
			c.Status(http.StatusNotFound)
			return
		}
	}
}

const contentType = "application/vnd.git-lfs+json"

// verifyAcceptHeader checks if the "Accept" header is "application/vnd.git-lfs+json".
func verifyAcceptHeader() macaron.Handler {
	return func(c *context.Context) {
		if c.Req.Header.Get("Accept") != contentType {
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

	// TODO
}

func serveBatchUpload() {

}

func serveBatchDownload() {

}

func serveBatchVerify() {

}
