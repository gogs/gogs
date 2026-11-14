package repo

import (
	"net/http"

	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database"
)

func TriggerTask(logger *logx.Logger, c flamego.Context) {
	branch := c.Query("branch")
	pusherID := c.QueryInt64("pusher")
	secret := c.Query("secret")
	if branch == "" || pusherID <= 0 || secret == "" {
		writeError(c.ResponseWriter(), http.StatusBadRequest, "Incomplete branch, pusher or secret")
		return
	}

	username := c.Param("owner")
	reponame := c.Param("repo")

	owner, err := database.Handle.Users().GetByUsername(c.Request().Context(), username)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeError(c.ResponseWriter(), http.StatusBadRequest, "Owner does not exist")
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to get user", "userName", username, "error", err)
		}
		return
	}

	// 🚨 SECURITY: No need to check existence of the repository if the client
	// can't even get the valid secret. Mostly likely not a legitimate request.
	if secret != cryptox.MD5(owner.Salt) {
		writeError(c.ResponseWriter(), http.StatusBadRequest, "Invalid secret")
		return
	}

	repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), owner.ID, reponame)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			writeError(c.ResponseWriter(), http.StatusBadRequest, "Repository does not exist")
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to get repository", "ownerID", owner.ID, "repoName", reponame, "error", err)
		}
		return
	}

	pusher, err := database.Handle.Users().GetByID(c.Request().Context(), pusherID)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeError(c.ResponseWriter(), http.StatusBadRequest, "Pusher does not exist")
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to get user", "userID", pusherID, "error", err)
		}
		return
	}

	logger.Debug("Task triggered", "ownerName", owner.Name, "repoName", repo.Name, "branch", branch, "pusherName", pusher.Name)

	database.TriggerPushTasks(logger, pusher, repo.ID, branch)
	c.ResponseWriter().WriteHeader(http.StatusAccepted)
}
