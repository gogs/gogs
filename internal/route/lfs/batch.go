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
	"gogs.io/gogs/internal/lfsutil"
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

// verifyAcceptHeader checks if the "Accept" header is "application/vnd.git-lfs+json".
func verifyAcceptHeader() macaron.Handler {
	return func(c *context.Context) {
		if c.Req.Header.Get("Accept") != lfsutil.ContentType {
			c.Status(http.StatusNotAcceptable)
			return
		}
	}
}

func serveBatch(c *context.Context, owner *db.User, repo *db.Repository) {
	var request lfsutil.BatchRequest
	defer c.Req.Request.Body.Close()
	err := json.NewDecoder(c.Req.Request.Body).Decode(&request)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	objects := make([]lfsutil.BatchObject, 0, len(request.Objects))
	for _, obj := range request.Objects {
		var actions lfsutil.BatchActions
		action := &lfsutil.BatchAction{
			Href: fmt.Sprintf("%s%s/%s.git/info/lfs/objects/batch/%s", conf.Server.ExternalURL, owner.Name, repo.Name, obj.Oid),
		}
		switch request.Operation {
		case lfsutil.BatchOperationUpload:
			actions.Upload = action
		case lfsutil.BatchOperationDownload:
			// TODO: Check if object exists
			actions.Download = action
		default:
			actions.Error = &lfsutil.BatchError{
				Code:    http.StatusUnprocessableEntity,
				Message: "Operation not recognized",
			}
		}

		objects = append(objects, lfsutil.BatchObject{
			Oid:     obj.Oid,
			Size:    obj.Size,
			Actions: actions,
		})
	}

	c.JSONSuccess(lfsutil.BatchResponse{
		Transfer: lfsutil.TransferBasic,
		Objects:  objects,
	})
}

func serveBatchUpload() {

}

func serveBatchDownload() {

}

func serveBatchVerify() {

}
