package repo

import (
	"net/http"

	"gopkg.in/macaron.v1"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database"
)

func TriggerTask(logger *logx.Logger, c *macaron.Context) {
	branch := c.Query("branch")
	pusherID := c.QueryInt64("pusher")
	secret := c.Query("secret")
	if branch == "" || pusherID <= 0 || secret == "" {
		c.Error(http.StatusBadRequest, "Incomplete branch, pusher or secret")
		return
	}

	username := c.Params(":username")
	reponame := c.Params(":reponame")

	owner, err := database.Handle.Users().GetByUsername(c.Req.Context(), username)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Error(http.StatusBadRequest, "Owner does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			logger.Error("Failed to get user", "userName", username, "error", err)
		}
		return
	}

	// 🚨 SECURITY: No need to check existence of the repository if the client
	// can't even get the valid secret. Mostly likely not a legitimate request.
	if secret != cryptox.MD5(owner.Salt) {
		c.Error(http.StatusBadRequest, "Invalid secret")
		return
	}

	repo, err := database.Handle.Repositories().GetByName(c.Req.Context(), owner.ID, reponame)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			c.Error(http.StatusBadRequest, "Repository does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			logger.Error("Failed to get repository", "ownerID", owner.ID, "repoName", reponame, "error", err)
		}
		return
	}

	pusher, err := database.Handle.Users().GetByID(c.Req.Context(), pusherID)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Error(http.StatusBadRequest, "Pusher does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			logger.Error("Failed to get user", "userID", pusherID, "error", err)
		}
		return
	}

	logger.Debug("Task triggered", "ownerName", owner.Name, "repoName", repo.Name, "branch", branch, "pusherName", pusher.Name)

	go database.HookQueue.Add(repo.ID)
	go database.AddTestPullRequestTask(logger, pusher, repo.ID, branch, true)
	c.Status(http.StatusAccepted)
}
