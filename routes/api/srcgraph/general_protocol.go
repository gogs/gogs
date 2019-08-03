// Copyright 2019 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package srcgraph

import (
	"github.com/Unknwon/com"
	"net/http"
	"time"

	adapter "github.com/sourcegraph/external-service-adapter"
	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/setting"
)

func NewHandler() http.HandlerFunc {
	h := adapter.NewHandler(externalServicer{}, adapter.Options{
		URL:        setting.AppURL,
		PathPrefix: "/-/srcgraph",
		MaxPageLen: 100000, // Current version returns all repositories at once, does not matter
	})
	return h.ServeHTTP
}

type externalServicer struct{}

func (es externalServicer) ListRepos(ai adapter.AuthInfo, params adapter.Params) ([]*adapter.Repo, adapter.Page, error) {
	return es.listUserRepos("", ai, params)
}

func (es externalServicer) ListUserRepos(user string, ai adapter.AuthInfo, params adapter.Params) ([]*adapter.Repo, adapter.Page, error) {
	return es.listUserRepos(user, ai, params)
}

func toRepo(r *models.Repository) *adapter.Repo {
	var parent *adapter.Repo
	if r.IsFork {
		parent = toRepo(r.BaseRepo)
	}

	cl := r.CloneLink()
	return &adapter.Repo{
		ID:          com.ToStr(r.ID),
		Name:        r.Name,
		Slug:        r.Name,
		FullName:    r.FullName(),
		SCM:         "git",
		Description: r.Description,
		IsPrivate:   r.IsPrivate,
		Parent:      parent,
		Links: []adapter.Link{
			{adapter.CloneSSH, cl.SSH},
			{adapter.CloneHTTP, cl.HTTPS},
		},
	}
}

func (es externalServicer) listUserRepos(username string, ai adapter.AuthInfo, params adapter.Params) ([]*adapter.Repo, adapter.Page, error) {
	authUser, err := userFromAuthInfo(ai)
	if err != nil {
		if errors.IsUserNotExist(err) {
			return nil, adapter.Page{}, errors.New("403 Forbidden")
		}
		log.Error(2, "Failed to get user from auth info: %v", err)
		return nil, adapter.Page{}, errors.New("500 Internal Server Error")
	}

	// Fall back to authenticated user
	if username == "" {
		username = authUser.Name
	}

	user, err := models.GetUserByName(username)
	if err != nil {
		if errors.IsUserNotExist(err) {
			return nil, adapter.Page{}, errors.New("404 Not Found")
		}
		log.Error(2, "Failed to get user by username %q: %v", username, err)
		return nil, adapter.Page{}, errors.New("500 Internal Server Error")
	}

	// Only list public repositories if user requests someone else's repository list,
	// or an organization isn't a member of.
	var ownRepos []*models.Repository
	if user.IsOrganization() {
		ownRepos, _, err = user.GetUserRepositories(authUser.ID, params.Page, user.NumRepos)
	} else {
		ownRepos, err = models.GetUserRepositories(&models.UserRepoOptions{
			UserID:   user.ID,
			Private:  authUser.ID == user.ID,
			Page:     params.Page,
			PageSize: user.NumRepos,
		})
	}
	if err != nil {
		log.Error(2, "Failed to get repositories of user %q: %v", username, err)
		return nil, adapter.Page{}, errors.New("500 Internal Server Error")
	}

	if err = models.RepositoryList(ownRepos).LoadAttributes(); err != nil {
		log.Error(2, "Failed to load attributes of repositories: %v", err)
		return nil, adapter.Page{}, errors.New("500 Internal Server Error")
	}

	// Early return for querying other user's repositories
	if authUser.ID != user.ID {
		repos := make([]*adapter.Repo, len(ownRepos))
		for i := range ownRepos {
			repos[i] = toRepo(ownRepos[i])
		}
		return repos, adapter.Page{Last: 1}, nil
	}

	accessibleRepos, err := user.GetRepositoryAccesses()
	if err != nil {
		log.Error(2, "Failed to get accessible repositories of user %q: %v", username, err)
		return nil, adapter.Page{}, errors.New("500 Internal Server Error")
	}

	numOwnRepos := len(ownRepos)
	repos := make([]*adapter.Repo, numOwnRepos+len(accessibleRepos))
	for i := range ownRepos {
		repos[i] = toRepo(ownRepos[i])
	}

	i := numOwnRepos
	for repo := range accessibleRepos {
		repos[i] = toRepo(repo)
		i++
	}

	return repos, adapter.Page{Last: 1}, nil
}

func userFromAuthInfo(ai adapter.AuthInfo) (*models.User, error) {
	u, err := models.UserLogin(ai.Username, ai.Password, -1)
	if err != nil && !errors.IsUserNotExist(err) {
		return nil, err
	}

	if u != nil {
		if u.IsEnabledTwoFactor() {
			return nil, errors.New(
				"User with two-factor authentication enabled cannot perform HTTP/HTTPS operations via plain username and password." +
					" Please create and use personal access token on user settings page.")
		}
		return u, nil
	}

	t, err := models.GetAccessTokenBySHA(ai.Username)
	if err != nil {
		if models.IsErrAccessTokenEmpty(err) || models.IsErrAccessTokenNotExist(err) {
			return nil, errors.UserNotExist{}
		}
		return nil, err
	}
	t.Updated = time.Now()

	u, err = models.GetUserByID(t.UID)
	if err != nil {
		return nil, err
	}

	return u, models.UpdateAccessToken(t)
}
