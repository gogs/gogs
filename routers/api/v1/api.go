// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"strings"

	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers/api/v1/admin"
	"github.com/gogits/gogs/routers/api/v1/misc"
	"github.com/gogits/gogs/routers/api/v1/org"
	"github.com/gogits/gogs/routers/api/v1/repo"
	"github.com/gogits/gogs/routers/api/v1/user"
)

func RepoAssignment() macaron.Handler {
	return func(ctx *middleware.Context) {
		userName := ctx.Params(":username")
		repoName := ctx.Params(":reponame")

		var (
			owner *models.User
			err   error
		)

		// Check if the user is the same as the repository owner.
		if ctx.IsSigned && ctx.User.LowerName == strings.ToLower(userName) {
			owner = ctx.User
		} else {
			owner, err = models.GetUserByName(userName)
			if err != nil {
				if models.IsErrUserNotExist(err) {
					ctx.Error(404)
				} else {
					ctx.APIError(500, "GetUserByName", err)
				}
				return
			}
		}
		ctx.Repo.Owner = owner

		// Get repository.
		repo, err := models.GetRepositoryByName(owner.Id, repoName)
		if err != nil {
			if models.IsErrRepoNotExist(err) {
				ctx.Error(404)
			} else {
				ctx.APIError(500, "GetRepositoryByName", err)
			}
			return
		} else if err = repo.GetOwner(); err != nil {
			ctx.APIError(500, "GetOwner", err)
			return
		}

		mode, err := models.AccessLevel(ctx.User, repo)
		if err != nil {
			ctx.APIError(500, "AccessLevel", err)
			return
		}

		ctx.Repo.AccessMode = mode

		// Check access.
		if ctx.Repo.AccessMode == models.ACCESS_MODE_NONE {
			ctx.Error(404)
			return
		}

		ctx.Repo.Repository = repo
	}
}

// Contexter middleware already checks token for user sign in process.
func ReqToken() macaron.Handler {
	return func(ctx *middleware.Context) {
		if !ctx.IsSigned {
			ctx.Error(401)
			return
		}
	}
}

func ReqBasicAuth() macaron.Handler {
	return func(ctx *middleware.Context) {
		if !ctx.IsBasicAuth {
			ctx.Error(401)
			return
		}
	}
}

func ReqAdmin() macaron.Handler {
	return func(ctx *middleware.Context) {
		if !ctx.User.IsAdmin {
			ctx.Error(403)
			return
		}
	}
}

// RegisterRoutes registers all v1 APIs routes to web application.
// FIXME: custom form error response
func RegisterRoutes(m *macaron.Macaron) {
	bind := binding.Bind

	m.Group("/v1", func() {
		// Miscellaneous
		m.Post("/markdown", bind(api.MarkdownOption{}), misc.Markdown)
		m.Post("/markdown/raw", misc.MarkdownRaw)

		// Users
		m.Group("/users", func() {
			m.Get("/search", user.Search)

			m.Group("/:username", func() {
				m.Get("", user.GetInfo)

				m.Group("/tokens", func() {
					m.Combo("").Get(user.ListAccessTokens).
						Post(bind(api.CreateAccessTokenOption{}), user.CreateAccessToken)
				}, ReqBasicAuth())
			})
		})

		m.Group("/users", func() {
			m.Group("/:username", func() {
				m.Get("/keys", user.ListPublicKeys)
			})
		}, ReqToken())

		m.Group("/user", func() {
			m.Group("/keys", func() {
				m.Combo("").Get(user.ListMyPublicKeys).
					Post(bind(api.CreateKeyOption{}), user.CreatePublicKey)
				m.Combo("/:id").Get(user.GetPublicKey).
					Delete(user.DeletePublicKey)
			})
			m.Combo("/emails").Get(user.ListEmails).
				Post(bind(api.CreateEmailOption{}), user.AddEmail).
				Delete(bind(api.CreateEmailOption{}), user.DeleteEmail)
		}, ReqToken())

		// Repositories
		m.Combo("/user/repos", ReqToken()).Get(repo.ListMyRepos).
			Post(bind(api.CreateRepoOption{}), repo.Create)
		m.Post("/org/:org/repos", ReqToken(), bind(api.CreateRepoOption{}), repo.CreateOrgRepo)

		m.Group("/repos", func() {
			m.Get("/search", repo.Search)
		})

		m.Group("/repos", func() {
			m.Post("/migrate", bind(auth.MigrateRepoForm{}), repo.Migrate)
			m.Combo("/:username/:reponame").Get(repo.Get).
				Delete(repo.Delete)

			m.Group("/:username/:reponame", func() {
				m.Combo("/hooks").Get(repo.ListHooks).
					Post(bind(api.CreateHookOption{}), repo.CreateHook)
				m.Patch("/hooks/:id:int", bind(api.EditHookOption{}), repo.EditHook)
				m.Get("/raw/*", middleware.RepoRef(), repo.GetRawFile)
				m.Get("/archive/*", repo.GetArchive)

				m.Group("/keys", func() {
					m.Combo("").Get(repo.ListDeployKeys).
						Post(bind(api.CreateKeyOption{}), repo.CreateDeployKey)
					m.Combo("/:id").Get(repo.GetDeployKey).
						Delete(repo.DeleteDeploykey)
				})
			}, RepoAssignment())
		}, ReqToken())

		// Organizations
		m.Get("/user/orgs", org.ListMyOrgs)
		m.Get("/users/:username/orgs", org.ListUserOrgs)
		m.Combo("/orgs/:orgname").Get(org.Get).Patch(bind(api.EditOrgOption{}), org.Edit)

		m.Any("/*", func(ctx *middleware.Context) {
			ctx.Error(404)
		})

		m.Group("/admin", func() {
			m.Group("/users", func() {
				m.Post("", bind(api.CreateUserOption{}), admin.CreateUser)

				m.Group("/:username", func() {
					m.Combo("").Patch(bind(api.EditUserOption{}), admin.EditUser).
						Delete(admin.DeleteUser)
					m.Post("/keys", admin.CreatePublicKey)
					m.Post("/orgs", bind(api.CreateOrgOption{}), admin.CreateOrg)
					m.Post("/repos", bind(api.CreateRepoOption{}), admin.CreateRepo)
				})
			})
		}, ReqAdmin())
	})
}
