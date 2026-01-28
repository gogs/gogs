package repo

import (
	"net/http"

	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/database"
)

func TriggerTask(c flamego.Context) {
	branch := c.Query("branch")
	pusherID := c.QueryInt64("pusher")
	secret := c.Query("secret")
	if branch == "" || pusherID <= 0 || secret == "" {
		c.ResponseWriter().WriteHeader(http.StatusBadRequest)
		c.ResponseWriter().Write([]byte("Incomplete branch, pusher or secret"))
		return
	}

	username := c.Param("username")
	reponame := c.Param("reponame")

	owner, err := database.Handle.Users().GetByUsername(c.Request().Context(), username)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.ResponseWriter().WriteHeader(http.StatusBadRequest)
			c.ResponseWriter().Write([]byte("Owner does not exist"))
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			log.Error("Failed to get user [name: %s]: %v", username, err)
		}
		return
	}

	// 🚨 SECURITY: No need to check existence of the repository if the client
	// can't even get the valid secret. Mostly likely not a legitimate request.
	if secret != cryptoutil.MD5(owner.Salt) {
		c.ResponseWriter().WriteHeader(http.StatusBadRequest)
		c.ResponseWriter().Write([]byte("Invalid secret"))
		return
	}

	repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), owner.ID, reponame)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			c.ResponseWriter().WriteHeader(http.StatusBadRequest)
			c.ResponseWriter().Write([]byte("Repository does not exist"))
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			log.Error("Failed to get repository [owner_id: %d, name: %s]: %v", owner.ID, reponame, err)
		}
		return
	}

	pusher, err := database.Handle.Users().GetByID(c.Request().Context(), pusherID)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.ResponseWriter().WriteHeader(http.StatusBadRequest)
			c.ResponseWriter().Write([]byte("Pusher does not exist"))
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			log.Error("Failed to get user [id: %d]: %v", pusherID, err)
		}
		return
	}

	log.Trace("TriggerTask: %s/%s@%s by %q", owner.Name, repo.Name, branch, pusher.Name)

	go database.HookQueue.Add(repo.ID)
	go database.AddTestPullRequestTask(pusher, repo.ID, branch, true)
	c.ResponseWriter().WriteHeader(http.StatusAccepted)
}
