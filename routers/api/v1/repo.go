// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"fmt"
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
		Id:          repo.Id,
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
			u, err := models.GetUserById(opt.Uid)
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
			Id:       repos[i].Id,
			FullName: path.Join(repos[i].Owner.Name, repos[i].Name),
		}
	}

	ctx.JSON(200, map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}

func createRepo(ctx *middleware.Context, owner *models.User, opt api.CreateRepoOption) {
	repo, err := models.CreateRepository(owner, opt.Name, opt.Description,
		opt.Gitignore, opt.License, opt.Private, false, opt.AutoInit)
	if err != nil {
		if err == models.ErrRepoAlreadyExist ||
			err == models.ErrRepoNameIllegal {
			ctx.JSON(422, &base.ApiJsonErr{err.Error(), base.DOC_URL})
		} else {
			log.Error(4, "CreateRepository: %v", err)
			if repo != nil {
				if err = models.DeleteRepository(ctx.User.Id, repo.Id, ctx.User.Name); err != nil {
					log.Error(4, "DeleteRepository: %v", err)
				}
			}
			ctx.Error(500)
		}
		return
	}

	ctx.JSON(200, ToApiRepository(owner, repo, api.Permission{true, true, true}))
}

// POST /user/repos
// https://developer.github.com/v3/repos/#create
func CreateRepo(ctx *middleware.Context, opt api.CreateRepoOption) {
	// Shouldn't reach this condition, but just in case.
	if ctx.User.IsOrganization() {
		ctx.JSON(422, "not allowed creating repository for organization")
		return
	}
	createRepo(ctx, ctx.User, opt)
}

// POST /orgs/:org/repos
// https://developer.github.com/v3/repos/#create
func CreateOrgRepo(ctx *middleware.Context, opt api.CreateRepoOption) {
	org, err := models.GetOrgByName(ctx.Params(":org"))
	if err != nil {
		if err == models.ErrUserNotExist {
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
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	if !u.ValidtePassword(ctx.Query("password")) {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": "username or password is not correct",
		})
		return
	}

	ctxUser := u
	// Not equal means current user is an organization.
	if form.Uid != u.Id {
		org, err := models.GetUserById(form.Uid)
		if err != nil {
			log.Error(4, "GetUserById: %v", err)
			ctx.Error(500)
			return
		}
		ctxUser = org
	}

	if ctx.HasError() {
		ctx.JSON(422, map[string]interface{}{
			"ok":    false,
			"error": ctx.GetErrMsg(),
		})
		return
	}

	if ctxUser.IsOrganization() {
		// Check ownership of organization.
		if !ctxUser.IsOwnedBy(u.Id) {
			ctx.JSON(403, map[string]interface{}{
				"ok":    false,
				"error": "given user is not owner of organization",
			})
			return
		}
	}

	authStr := strings.Replace(fmt.Sprintf("://%s:%s",
		form.AuthUserName, form.AuthPasswd), "@", "%40", -1)
	url := strings.Replace(form.HttpsUrl, "://", authStr+"@", 1)
	repo, err := models.MigrateRepository(ctxUser, form.RepoName, form.Description, form.Private,
		form.Mirror, url)
	if err == nil {
		log.Trace("Repository migrated: %s/%s", ctxUser.Name, form.RepoName)
		ctx.JSON(200, map[string]interface{}{
			"ok":   true,
			"data": "/" + ctxUser.Name + "/" + form.RepoName,
		})
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.Id, repo.Id, ctxUser.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	ctx.JSON(500, map[string]interface{}{
		"ok":    false,
		"error": err.Error(),
	})
}

// GET /user/repos
// https://developer.github.com/v3/repos/#list-your-repositories
func ListMyRepos(ctx *middleware.Context) {
	ownRepos, err := models.GetRepositories(ctx.User.Id, true)
	if err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"GetRepositories: " + err.Error(), base.DOC_URL})
		return
	}
	numOwnRepos := len(ownRepos)

	collaRepos, err := models.GetCollaborativeRepos(ctx.User.Name)
	if err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"GetCollaborativeRepos: " + err.Error(), base.DOC_URL})
		return
	}

	repos := make([]*api.Repository, numOwnRepos+len(collaRepos))
	for i := range ownRepos {
		repos[i] = ToApiRepository(ctx.User, ownRepos[i], api.Permission{true, true, true})
	}
	for i := range collaRepos {
		if err = collaRepos[i].GetOwner(); err != nil {
			ctx.JSON(500, &base.ApiJsonErr{"GetOwner: " + err.Error(), base.DOC_URL})
			return
		}
		j := i + numOwnRepos
		repos[j] = ToApiRepository(collaRepos[i].Owner, collaRepos[i].Repository, api.Permission{false, collaRepos[i].CanPush, true})

		// FIXME: cache result to reduce DB query?
		if collaRepos[i].Owner.IsOrganization() && collaRepos[i].Owner.IsOwnedBy(ctx.User.Id) {
			repos[j].Permissions.Admin = true
		}
	}

	ctx.JSON(200, &repos)
}
