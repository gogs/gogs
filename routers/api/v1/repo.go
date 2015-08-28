// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"net/url"
	"path"
	"strings"

	"github.com/Unknwon/com"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

// ToApiRepository converts repository to API format.
func ToApiRepository(owner *models.User, repo *models.Repository, permission api.Permission) *api.Repository {
	cl, err := repo.CloneLink()
	if err != nil {
		log.Error(4, "CloneLink: %v", err)
	}
	return &api.Repository{
		Id:          repo.ID,
		Owner:       *ToApiUser(owner),
		FullName:    owner.Name + "/" + repo.Name,
		Private:     repo.IsPrivate,
		Fork:        repo.IsFork,
		HtmlUrl:     setting.AppUrl + owner.Name + "/" + repo.Name,
		CloneUrl:    cl.HTTPS,
		SshUrl:      cl.SSH,
		Permissions: permission,
	}
}

func SearchRepos(ctx *middleware.Context) {
	opt := models.SearchOption{
		Keyword: path.Base(ctx.Query("q")),
		Uid:     com.StrTo(ctx.Query("uid")).MustInt64(),
		Limit:   com.StrTo(ctx.Query("limit")).MustInt(),
	}
	if opt.Limit == 0 {
		opt.Limit = 10
	}

	// Check visibility.
	if ctx.IsSigned && opt.Uid > 0 {
		if ctx.User.Id == opt.Uid {
			opt.Private = true
		} else {
			u, err := models.GetUserByID(opt.Uid)
			if err != nil {
				ctx.JSON(500, map[string]interface{}{
					"ok":    false,
					"error": err.Error(),
				})
				return
			}
			if u.IsOrganization() && u.IsOwnedBy(ctx.User.Id) {
				opt.Private = true
			}
			// FIXME: how about collaborators?
		}
	}

	repos, err := models.SearchRepositoryByName(opt)
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*api.Repository, len(repos))
	for i := range repos {
		if err = repos[i].GetOwner(); err != nil {
			ctx.JSON(500, map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		results[i] = &api.Repository{
			Id:       repos[i].ID,
			FullName: path.Join(repos[i].Owner.Name, repos[i].Name),
		}
	}

	ctx.JSON(200, map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}

// https://github.com/gogits/go-gogs-client/wiki/Repositories#list-your-repositories
func ListMyRepos(ctx *middleware.Context) {
	ownRepos, err := models.GetRepositories(ctx.User.Id, true)
	if err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"GetRepositories: " + err.Error(), base.DOC_URL})
		return
	}
	numOwnRepos := len(ownRepos)

	accessibleRepos, err := ctx.User.GetAccessibleRepositories()
	if err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"GetAccessibleRepositories: " + err.Error(), base.DOC_URL})
		return
	}

	repos := make([]*api.Repository, numOwnRepos+len(accessibleRepos))
	for i := range ownRepos {
		repos[i] = ToApiRepository(ctx.User, ownRepos[i], api.Permission{true, true, true})
	}
	i := numOwnRepos

	for repo, access := range accessibleRepos {
		repos[i] = ToApiRepository(repo.Owner, repo, api.Permission{
			Admin: access >= models.ACCESS_MODE_ADMIN,
			Push:  access >= models.ACCESS_MODE_WRITE,
			Pull:  true,
		})
		i++
	}

	ctx.JSON(200, &repos)
}

func createRepo(ctx *middleware.Context, owner *models.User, opt api.CreateRepoOption) {
	repo, err := models.CreateRepository(owner, models.CreateRepoOptions{
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
			ctx.JSON(422, &base.ApiJsonErr{err.Error(), base.DOC_URL})
		} else {
			log.Error(4, "CreateRepository: %v", err)
			if repo != nil {
				if err = models.DeleteRepository(ctx.User.Id, repo.ID, ctx.User.Name); err != nil {
					log.Error(4, "DeleteRepository: %v", err)
				}
			}
			ctx.Error(500)
		}
		return
	}

	ctx.JSON(201, ToApiRepository(owner, repo, api.Permission{true, true, true}))
}

// https://github.com/gogits/go-gogs-client/wiki/Repositories#create
func CreateRepo(ctx *middleware.Context, opt api.CreateRepoOption) {
	// Shouldn't reach this condition, but just in case.
	if ctx.User.IsOrganization() {
		ctx.JSON(422, "not allowed creating repository for organization")
		return
	}
	createRepo(ctx, ctx.User, opt)
}

func CreateOrgRepo(ctx *middleware.Context, opt api.CreateRepoOption) {
	org, err := models.GetOrgByName(ctx.Params(":org"))
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Error(404)
		} else {
			ctx.Error(500)
		}
		return
	}

	if !org.IsOwnedBy(ctx.User.Id) {
		ctx.Error(403)
		return
	}
	createRepo(ctx, org, opt)
}

func MigrateRepo(ctx *middleware.Context, form auth.MigrateRepoForm) {
	u, err := models.GetUserByName(ctx.Query("username"))
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.HandleAPI(422, err)
		} else {
			ctx.HandleAPI(500, err)
		}
		return
	}
	if !u.ValidatePassword(ctx.Query("password")) {
		ctx.HandleAPI(422, "Username or password is not correct.")
		return
	}

	ctxUser := u
	// Not equal means current user is an organization.
	if form.Uid != u.Id {
		org, err := models.GetUserByID(form.Uid)
		if err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.HandleAPI(422, err)
			} else {
				ctx.HandleAPI(500, err)
			}
			return
		}
		ctxUser = org
	}

	if ctx.HasError() {
		ctx.HandleAPI(422, ctx.GetErrMsg())
		return
	}

	if ctxUser.IsOrganization() {
		// Check ownership of organization.
		if !ctxUser.IsOwnedBy(u.Id) {
			ctx.HandleAPI(403, "Given user is not owner of organization.")
			return
		}
	}

	// Remote address can be HTTP/HTTPS/Git URL or local path.
	remoteAddr := form.CloneAddr
	if strings.HasPrefix(form.CloneAddr, "http://") ||
		strings.HasPrefix(form.CloneAddr, "https://") ||
		strings.HasPrefix(form.CloneAddr, "git://") {
		u, err := url.Parse(form.CloneAddr)
		if err != nil {
			ctx.HandleAPI(422, err)
			return
		}
		if len(form.AuthUsername) > 0 || len(form.AuthPassword) > 0 {
			u.User = url.UserPassword(form.AuthUsername, form.AuthPassword)
		}
		remoteAddr = u.String()
	} else if !com.IsDir(remoteAddr) {
		ctx.HandleAPI(422, "Invalid local path, it does not exist or not a directory.")
		return
	}

	repo, err := models.MigrateRepository(ctxUser, form.RepoName, form.Description, form.Private, form.Mirror, remoteAddr)
	if err != nil {
		if repo != nil {
			if errDelete := models.DeleteRepository(ctxUser.Id, repo.ID, ctxUser.Name); errDelete != nil {
				log.Error(4, "DeleteRepository: %v", errDelete)
			}
		}
		ctx.HandleAPI(500, err)
		return
	}

	log.Trace("Repository migrated: %s/%s", ctxUser.Name, form.RepoName)
	ctx.WriteHeader(200)
}
