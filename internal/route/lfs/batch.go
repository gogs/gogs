// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
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
				Put(authorize(db.AccessModeWrite), serveBatchUpload)
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

		c.Map(owner)
		c.Map(repo)
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

const transferBasic = "basic"
const (
	batchOperationUpload   = "upload"
	batchOperationDownload = "download"
)

func serveBatch(c *context.Context, owner *db.User, repo *db.Repository) {
	// TODO: Define types in lfsutil
	var request struct {
		Operation string `json:"operation"`
		Objects   []struct {
			Oid  string `json:"oid"`
			Size int    `json:"size"`
		} `json:"objects"`
	}

	defer c.Req.Request.Body.Close()
	err := json.NewDecoder(c.Req.Request.Body).Decode(&request)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	type error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	type action struct {
		Href string `json:"href"`
	}
	type actions struct {
		Download *action `json:"download,omitempty"`
		Upload   *action `json:"upload,omitempty"`
		Verify   *action `json:"verify,omitempty"`
		Error    *error  `json:"error,omitempty"`
	}
	type object struct {
		Oid     string  `json:"oid"`
		Size    int     `json:"size"`
		Actions actions `json:"actions"`
	}

	objects := make([]object, 0, len(request.Objects))
	for _, obj := range request.Objects {
		var actions actions
		action := &action{
			Href: fmt.Sprintf("%s%s/%s.git/info/lfs/objects/batch/%s", conf.Server.ExternalURL, owner.Name, repo.Name, obj.Oid),
		}
		switch request.Operation {
		case batchOperationUpload:
			actions.Upload = action
		case batchOperationDownload:
			// TODO: Check if object exists
			actions.Download = action
		default:
			actions.Error = &error{
				Code:    http.StatusUnprocessableEntity,
				Message: "Operation not recognized",
			}
		}

		objects = append(objects, object{
			Oid:     obj.Oid,
			Size:    obj.Size,
			Actions: actions,
		})
	}

	var response = struct {
		Transfer string   `json:"transfer"`
		Objects  []object `json:"objects"`
	}{
		Transfer: transferBasic,
		Objects:  objects,
	}

	c.JSONSuccess(response)
}

func serveBatchUpload() {

}

func serveBatchDownload() {

}

func serveBatchVerify() {

}
