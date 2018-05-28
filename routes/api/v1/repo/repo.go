// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"path"

	log "gopkg.in/clog.v1"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/routes/api/v1/convert"
)

// https://github.com/gogs/go-gogs-client/wiki/Repositories#search-repositories
func Search(c *context.APIContext) {
	opts := &models.SearchRepoOptions{
		Keyword:  path.Base(c.Query("q")),
		OwnerID:  c.QueryInt64("uid"),
		PageSize: convert.ToCorrectPageSize(c.QueryInt("limit")),
	}

	// Check visibility.
	if c.IsLogged && opts.OwnerID > 0 {
		if c.User.ID == opts.OwnerID {
			opts.Private = true
		} else {
			u, err := models.GetUserByID(opts.OwnerID)
			if err != nil {
				c.JSON(500, map[string]interface{}{
					"ok":    false,
					"error": err.Error(),
				})
				return
			}
			if u.IsOrganization() && u.IsOwnedBy(c.User.ID) {
				opts.Private = true
			}
			// FIXME: how about collaborators?
		}
	}

	repos, count, err := models.SearchRepositoryByName(opts)
	if err != nil {
		c.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if err = models.RepositoryList(repos).LoadAttributes(); err != nil {
		c.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*api.Repository, len(repos))
	for i := range repos {
		results[i] = repos[i].APIFormat(nil)
	}

	c.SetLinkHeader(int(count), setting.API.MaxResponseItems)
	c.JSON(200, map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}

func listUserRepositories(c *context.APIContext, username string) {
	user, err := models.GetUserByName(username)
	if err != nil {
		c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
		return
	}

	// Only list public repositories if user requests someone else's repository list,
	// or an organization isn't a member of.
	var ownRepos []*models.Repository
	if user.IsOrganization() {
		ownRepos, _, err = user.GetUserRepositories(c.User.ID, 1, user.NumRepos)
	} else {
		ownRepos, err = models.GetUserRepositories(&models.UserRepoOptions{
			UserID:   user.ID,
			Private:  c.User.ID == user.ID,
			Page:     1,
			PageSize: user.NumRepos,
		})
	}
	if err != nil {
		c.Error(500, "GetUserRepositories", err)
		return
	}

	if err = models.RepositoryList(ownRepos).LoadAttributes(); err != nil {
		c.Error(500, "LoadAttributes(ownRepos)", err)
		return
	}

	// Early return for querying other user's repositories
	if c.User.ID != user.ID {
		repos := make([]*api.Repository, len(ownRepos))
		for i := range ownRepos {
			repos[i] = ownRepos[i].APIFormat(&api.Permission{true, true, true})
		}
		c.JSON(200, &repos)
		return
	}

	accessibleRepos, err := user.GetRepositoryAccesses()
	if err != nil {
		c.Error(500, "GetRepositoryAccesses", err)
		return
	}

	numOwnRepos := len(ownRepos)
	repos := make([]*api.Repository, numOwnRepos+len(accessibleRepos))
	for i := range ownRepos {
		repos[i] = ownRepos[i].APIFormat(&api.Permission{true, true, true})
	}

	i := numOwnRepos
	for repo, access := range accessibleRepos {
		repos[i] = repo.APIFormat(&api.Permission{
			Admin: access >= models.ACCESS_MODE_ADMIN,
			Push:  access >= models.ACCESS_MODE_WRITE,
			Pull:  true,
		})
		i++
	}

	c.JSON(200, &repos)
}

func ListMyRepos(c *context.APIContext) {
	listUserRepositories(c, c.User.Name)
}

func ListUserRepositories(c *context.APIContext) {
	listUserRepositories(c, c.Params(":username"))
}

func ListOrgRepositories(c *context.APIContext) {
	listUserRepositories(c, c.Params(":org"))
}

func CreateUserRepo(c *context.APIContext, owner *models.User, opt api.CreateRepoOption) {
	repo, err := models.CreateRepository(c.User, owner, models.CreateRepoOptions{
		Name:        opt.Name,
		Description: opt.Description,
		Gitignores:  opt.Gitignores,
		License:     opt.License,
		Readme:      opt.Readme,
		IsPrivate:   opt.Private,
		AutoInit:    opt.AutoInit,
	})
	if err != nil {
		if models.IsErrRepoAlreadyExist(err) ||
			models.IsErrNameReserved(err) ||
			models.IsErrNamePatternNotAllowed(err) {
			c.Error(422, "", err)
		} else {
			if repo != nil {
				if err = models.DeleteRepository(c.User.ID, repo.ID); err != nil {
					log.Error(2, "DeleteRepository: %v", err)
				}
			}
			c.Error(500, "CreateRepository", err)
		}
		return
	}

	c.JSON(201, repo.APIFormat(&api.Permission{true, true, true}))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#create
func Create(c *context.APIContext, opt api.CreateRepoOption) {
	// Shouldn't reach this condition, but just in case.
	if c.User.IsOrganization() {
		c.Error(422, "", "not allowed creating repository for organization")
		return
	}
	CreateUserRepo(c, c.User, opt)
}

func CreateOrgRepo(c *context.APIContext, opt api.CreateRepoOption) {
	org, err := models.GetOrgByName(c.Params(":org"))
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "GetOrgByName", err)
		}
		return
	}

	if !org.IsOwnedBy(c.User.ID) {
		c.Error(403, "", "Given user is not owner of organization.")
		return
	}
	CreateUserRepo(c, org, opt)
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#migrate
func Migrate(c *context.APIContext, f form.MigrateRepo) {
	ctxUser := c.User
	// Not equal means context user is an organization,
	// or is another user/organization if current user is admin.
	if f.Uid != ctxUser.ID {
		org, err := models.GetUserByID(f.Uid)
		if err != nil {
			if errors.IsUserNotExist(err) {
				c.Error(422, "", err)
			} else {
				c.Error(500, "GetUserByID", err)
			}
			return
		} else if !org.IsOrganization() && !c.User.IsAdmin {
			c.Error(403, "", "Given user is not an organization")
			return
		}
		ctxUser = org
	}

	if c.HasError() {
		c.Error(422, "", c.GetErrMsg())
		return
	}

	if ctxUser.IsOrganization() && !c.User.IsAdmin {
		// Check ownership of organization.
		if !ctxUser.IsOwnedBy(c.User.ID) {
			c.Error(403, "", "Given user is not owner of organization")
			return
		}
	}

	remoteAddr, err := f.ParseRemoteAddr(c.User)
	if err != nil {
		if models.IsErrInvalidCloneAddr(err) {
			addrErr := err.(models.ErrInvalidCloneAddr)
			switch {
			case addrErr.IsURLError:
				c.Error(422, "", err)
			case addrErr.IsPermissionDenied:
				c.Error(422, "", "You are not allowed to import local repositories")
			case addrErr.IsInvalidPath:
				c.Error(422, "", "Invalid local path, it does not exist or not a directory")
			default:
				c.Error(500, "ParseRemoteAddr", "Unknown error type (ErrInvalidCloneAddr): "+err.Error())
			}
		} else {
			c.Error(500, "ParseRemoteAddr", err)
		}
		return
	}

	repo, err := models.MigrateRepository(c.User, ctxUser, models.MigrateRepoOptions{
		Name:        f.RepoName,
		Description: f.Description,
		IsPrivate:   f.Private || setting.Repository.ForcePrivate,
		IsMirror:    f.Mirror,
		RemoteAddr:  remoteAddr,
	})
	if err != nil {
		if repo != nil {
			if errDelete := models.DeleteRepository(ctxUser.ID, repo.ID); errDelete != nil {
				log.Error(2, "DeleteRepository: %v", errDelete)
			}
		}

		if errors.IsReachLimitOfRepo(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "MigrateRepository", models.HandleMirrorCredentials(err.Error(), true))
		}
		return
	}

	log.Trace("Repository migrated: %s/%s", ctxUser.Name, f.RepoName)
	c.JSON(201, repo.APIFormat(&api.Permission{true, true, true}))
}

func parseOwnerAndRepo(c *context.APIContext) (*models.User, *models.Repository) {
	owner, err := models.GetUserByName(c.Params(":username"))
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "GetUserByName", err)
		}
		return nil, nil
	}

	repo, err := models.GetRepositoryByName(owner.ID, c.Params(":reponame"))
	if err != nil {
		if errors.IsRepoNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetRepositoryByName", err)
		}
		return nil, nil
	}

	return owner, repo
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#get
func Get(c *context.APIContext) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	c.JSON(200, repo.APIFormat(&api.Permission{
		Admin: c.Repo.IsAdmin(),
		Push:  c.Repo.IsWriter(),
		Pull:  true,
	}))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#delete
func Delete(c *context.APIContext) {
	owner, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	if owner.IsOrganization() && !owner.IsOwnedBy(c.User.ID) {
		c.Error(403, "", "Given user is not owner of organization.")
		return
	}

	if err := models.DeleteRepository(owner.ID, repo.ID); err != nil {
		c.Error(500, "DeleteRepository", err)
		return
	}

	log.Trace("Repository deleted: %s/%s", owner.Name, repo.Name)
	c.Status(204)
}

func ListForks(c *context.APIContext) {
	forks, err := c.Repo.Repository.GetForks()
	if err != nil {
		c.Error(500, "GetForks", err)
		return
	}

	apiForks := make([]*api.Repository, len(forks))
	for i := range forks {
		if err := forks[i].GetOwner(); err != nil {
			c.Error(500, "GetOwner", err)
			return
		}
		apiForks[i] = forks[i].APIFormat(&api.Permission{
			Admin: c.User.IsAdminOfRepo(forks[i]),
			Push:  c.User.IsWriterOfRepo(forks[i]),
			Pull:  true,
		})
	}

	c.JSON(200, &apiForks)
}

func MirrorSync(c *context.APIContext) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	} else if !repo.IsMirror {
		c.Status(404)
		return
	}

	go models.MirrorQueue.Add(repo.ID)
	c.Status(202)
}
