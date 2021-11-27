// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"
	"path"

	api "github.com/gogs/go-gogs-client"
	"github.com/pkg/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/route/api/v1/convert"
)

func Search(c *context.APIContext) {
	opts := &db.SearchRepoOptions{
		Keyword:  path.Base(c.Query("q")),
		OwnerID:  c.QueryInt64("uid"),
		PageSize: convert.ToCorrectPageSize(c.QueryInt("limit")),
		Page:     c.QueryInt("page"),
	}

	// Check visibility.
	if c.IsLogged && opts.OwnerID > 0 {
		if c.User.ID == opts.OwnerID {
			opts.Private = true
		} else {
			u, err := db.GetUserByID(opts.OwnerID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, map[string]interface{}{
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

	repos, count, err := db.SearchRepositoryByName(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if err = db.RepositoryList(repos).LoadAttributes(); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*api.Repository, len(repos))
	for i := range repos {
		results[i] = repos[i].APIFormat(nil)
	}

	c.SetLinkHeader(int(count), opts.PageSize)
	c.JSONSuccess(map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}

func listUserRepositories(c *context.APIContext, username string) {
	user, err := db.GetUserByName(username)
	if err != nil {
		c.NotFoundOrError(err, "get user by name")
		return
	}

	// Only list public repositories if user requests someone else's repository list,
	// or an organization isn't a member of.
	var ownRepos []*db.Repository
	if user.IsOrganization() {
		ownRepos, _, err = user.GetUserRepositories(c.User.ID, 1, user.NumRepos)
	} else {
		ownRepos, err = db.GetUserRepositories(&db.UserRepoOptions{
			UserID:   user.ID,
			Private:  c.User.ID == user.ID,
			Page:     1,
			PageSize: user.NumRepos,
		})
	}
	if err != nil {
		c.Error(err, "get user repositories")
		return
	}

	if err = db.RepositoryList(ownRepos).LoadAttributes(); err != nil {
		c.Error(err, "load attributes")
		return
	}

	// Early return for querying other user's repositories
	if c.User.ID != user.ID {
		repos := make([]*api.Repository, len(ownRepos))
		for i := range ownRepos {
			repos[i] = ownRepos[i].APIFormat(&api.Permission{Admin: true, Push: true, Pull: true})
		}
		c.JSONSuccess(&repos)
		return
	}

	accessibleRepos, err := user.GetRepositoryAccesses()
	if err != nil {
		c.Error(err, "get repositories accesses")
		return
	}

	numOwnRepos := len(ownRepos)
	repos := make([]*api.Repository, numOwnRepos+len(accessibleRepos))
	for i := range ownRepos {
		repos[i] = ownRepos[i].APIFormat(&api.Permission{Admin: true, Push: true, Pull: true})
	}

	i := numOwnRepos
	for repo, access := range accessibleRepos {
		repos[i] = repo.APIFormat(&api.Permission{
			Admin: access >= db.AccessModeAdmin,
			Push:  access >= db.AccessModeWrite,
			Pull:  true,
		})
		i++
	}

	c.JSONSuccess(&repos)
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

func CreateUserRepo(c *context.APIContext, owner *db.User, opt api.CreateRepoOption) {
	repo, err := db.CreateRepository(c.User, owner, db.CreateRepoOptions{
		Name:        opt.Name,
		Description: opt.Description,
		Gitignores:  opt.Gitignores,
		License:     opt.License,
		Readme:      opt.Readme,
		IsPrivate:   opt.Private,
		AutoInit:    opt.AutoInit,
	})
	if err != nil {
		if db.IsErrRepoAlreadyExist(err) ||
			db.IsErrNameNotAllowed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			if repo != nil {
				if err = db.DeleteRepository(c.User.ID, repo.ID); err != nil {
					log.Error("Failed to delete repository: %v", err)
				}
			}
			c.Error(err, "create repository")
		}
		return
	}

	c.JSON(201, repo.APIFormat(&api.Permission{Admin: true, Push: true, Pull: true}))
}

func Create(c *context.APIContext, opt api.CreateRepoOption) {
	// Shouldn't reach this condition, but just in case.
	if c.User.IsOrganization() {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Not allowed to create repository for organization."))
		return
	}
	CreateUserRepo(c, c.User, opt)
}

func CreateOrgRepo(c *context.APIContext, opt api.CreateRepoOption) {
	org, err := db.GetOrgByName(c.Params(":org"))
	if err != nil {
		c.NotFoundOrError(err, "get organization by name")
		return
	}

	if !org.IsOwnedBy(c.User.ID) {
		c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not owner of organization."))
		return
	}
	CreateUserRepo(c, org, opt)
}

func Migrate(c *context.APIContext, f form.MigrateRepo) {
	ctxUser := c.User
	// Not equal means context user is an organization,
	// or is another user/organization if current user is admin.
	if f.Uid != ctxUser.ID {
		org, err := db.GetUserByID(f.Uid)
		if err != nil {
			if db.IsErrUserNotExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "get user by ID")
			}
			return
		} else if !org.IsOrganization() && !c.User.IsAdmin {
			c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not an organization."))
			return
		}
		ctxUser = org
	}

	if c.HasError() {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New(c.GetErrMsg()))
		return
	}

	if ctxUser.IsOrganization() && !c.User.IsAdmin {
		// Check ownership of organization.
		if !ctxUser.IsOwnedBy(c.User.ID) {
			c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not owner of organization."))
			return
		}
	}

	remoteAddr, err := f.ParseRemoteAddr(c.User)
	if err != nil {
		if db.IsErrInvalidCloneAddr(err) {
			addrErr := err.(db.ErrInvalidCloneAddr)
			switch {
			case addrErr.IsURLError:
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			case addrErr.IsPermissionDenied:
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("You are not allowed to import local repositories."))
			case addrErr.IsInvalidPath:
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Invalid local path, it does not exist or not a directory."))
			default:
				c.Error(err, "unexpected error")
			}
		} else {
			c.Error(err, "parse remote address")
		}
		return
	}

	repo, err := db.MigrateRepository(c.User, ctxUser, db.MigrateRepoOptions{
		Name:        f.RepoName,
		Description: f.Description,
		IsPrivate:   f.Private || conf.Repository.ForcePrivate,
		IsMirror:    f.Mirror,
		RemoteAddr:  remoteAddr,
	})
	if err != nil {
		if repo != nil {
			if errDelete := db.DeleteRepository(ctxUser.ID, repo.ID); errDelete != nil {
				log.Error("DeleteRepository: %v", errDelete)
			}
		}

		if db.IsErrReachLimitOfRepo(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(errors.New(db.HandleMirrorCredentials(err.Error(), true)), "migrate repository")
		}
		return
	}

	log.Trace("Repository migrated: %s/%s", ctxUser.Name, f.RepoName)
	c.JSON(201, repo.APIFormat(&api.Permission{Admin: true, Push: true, Pull: true}))
}

// FIXME: inject in the handler chain
func parseOwnerAndRepo(c *context.APIContext) (*db.User, *db.Repository) {
	owner, err := db.GetUserByName(c.Params(":username"))
	if err != nil {
		if db.IsErrUserNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get user by name")
		}
		return nil, nil
	}

	repo, err := db.GetRepositoryByName(owner.ID, c.Params(":reponame"))
	if err != nil {
		c.NotFoundOrError(err, "get repository by name")
		return nil, nil
	}

	return owner, repo
}

func Get(c *context.APIContext) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	c.JSONSuccess(repo.APIFormat(&api.Permission{
		Admin: c.Repo.IsAdmin(),
		Push:  c.Repo.IsWriter(),
		Pull:  true,
	}))
}

func Delete(c *context.APIContext) {
	owner, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	if owner.IsOrganization() && !owner.IsOwnedBy(c.User.ID) {
		c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not owner of organization."))
		return
	}

	if err := db.DeleteRepository(owner.ID, repo.ID); err != nil {
		c.Error(err, "delete repository")
		return
	}

	log.Trace("Repository deleted: %s/%s", owner.Name, repo.Name)
	c.NoContent()
}

func ListForks(c *context.APIContext) {
	forks, err := c.Repo.Repository.GetForks()
	if err != nil {
		c.Error(err, "get forks")
		return
	}

	apiForks := make([]*api.Repository, len(forks))
	for i := range forks {
		if err := forks[i].GetOwner(); err != nil {
			c.Error(err, "get owner")
			return
		}
		apiForks[i] = forks[i].APIFormat(&api.Permission{
			Admin: c.User.IsAdminOfRepo(forks[i]),
			Push:  c.User.IsWriterOfRepo(forks[i]),
			Pull:  true,
		})
	}

	c.JSONSuccess(&apiForks)
}

func IssueTracker(c *context.APIContext, form api.EditIssueTrackerOption) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	if form.EnableIssues != nil {
		repo.EnableIssues = *form.EnableIssues
	}
	if form.EnableExternalTracker != nil {
		repo.EnableExternalTracker = *form.EnableExternalTracker
	}
	if form.ExternalTrackerURL != nil {
		repo.ExternalTrackerURL = *form.ExternalTrackerURL
	}
	if form.TrackerURLFormat != nil {
		repo.ExternalTrackerFormat = *form.TrackerURLFormat
	}
	if form.TrackerIssueStyle != nil {
		repo.ExternalTrackerStyle = *form.TrackerIssueStyle
	}

	if err := db.UpdateRepository(repo, false); err != nil {
		c.Error(err, "update repository")
		return
	}

	c.NoContent()
}

func MirrorSync(c *context.APIContext) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	} else if !repo.IsMirror {
		c.NotFound()
		return
	}

	go db.MirrorQueue.Add(repo.ID)
	c.Status(http.StatusAccepted)
}

func Releases(c *context.APIContext) {
	_, repo := parseOwnerAndRepo(c)
	releases, err := db.GetReleasesByRepoID(repo.ID)
	if err != nil {
		c.Error(err, "get releases by repository ID")
		return
	}
	apiReleases := make([]*api.Release, 0, len(releases))
	for _, r := range releases {
		publisher, err := db.GetUserByID(r.PublisherID)
		if err != nil {
			c.Error(err, "get release publisher")
			return
		}
		r.Publisher = publisher
	}
	for _, r := range releases {
		apiReleases = append(apiReleases, r.APIFormat())
	}

	c.JSONSuccess(&apiReleases)
}
