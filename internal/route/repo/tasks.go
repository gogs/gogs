// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/database"
)

func TriggerTask(c *macaron.Context) {
	branch := c.Query("branch")
	pusherID := c.QueryInt64("pusher")
	secret := c.Query("secret")
	if branch == "" || pusherID <= 0 || secret == "" {
		c.Error(http.StatusBadRequest, "Incomplete branch, pusher or secret")
		return
	}

	username := c.Params(":username")
	reponame := c.Params(":reponame")

	owner, err := database.Users.GetByUsername(c.Req.Context(), username)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Error(http.StatusBadRequest, "Owner does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			log.Error("Failed to get user [name: %s]: %v", username, err)
		}
		return
	}

	// 🚨 SECURITY: No need to check existence of the repository if the client
	// can't even get the valid secret. Mostly likely not a legitimate request.
	if secret != cryptoutil.MD5(owner.Salt) {
		c.Error(http.StatusBadRequest, "Invalid secret")
		return
	}

	repo, err := database.Handle.Repositories().GetByName(c.Req.Context(), owner.ID, reponame)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			c.Error(http.StatusBadRequest, "Repository does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			log.Error("Failed to get repository [owner_id: %d, name: %s]: %v", owner.ID, reponame, err)
		}
		return
	}

	pusher, err := database.Users.GetByID(c.Req.Context(), pusherID)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Error(http.StatusBadRequest, "Pusher does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			log.Error("Failed to get user [id: %d]: %v", pusherID, err)
		}
		return
	}

	log.Trace("TriggerTask: %s/%s@%s by %q", owner.Name, repo.Name, branch, pusher.Name)

	go database.HookQueue.Add(repo.ID)
	go database.AddTestPullRequestTask(pusher, repo.ID, branch, true)
	c.Status(http.StatusAccepted)
}
