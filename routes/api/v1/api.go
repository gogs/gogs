// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"net/http"
	"strings"

	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/routes/api/v1/admin"
	"github.com/gogs/gogs/routes/api/v1/misc"
	"github.com/gogs/gogs/routes/api/v1/org"
	"github.com/gogs/gogs/routes/api/v1/repo"
	"github.com/gogs/gogs/routes/api/v1/user"
)

func repoAssignment() macaron.Handler {
	return func(c *context.APIContext) {
		userName := c.Params(":username")
		repoName := c.Params(":reponame")

		var (
			owner *models.User
			err   error
		)

		// Check if the user is the same as the repository owner.
		if c.IsLogged && c.User.LowerName == strings.ToLower(userName) {
			owner = c.User
		} else {
			owner, err = models.GetUserByName(userName)
			if err != nil {
				c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
				return
			}
		}
		c.Repo.Owner = owner

		repo, err := models.GetRepositoryByName(owner.ID, repoName)
		if err != nil {
			c.NotFoundOrServerError("GetRepositoryByName", errors.IsRepoNotExist, err)
			return
		} else if err = repo.GetOwner(); err != nil {
			c.ServerError("GetOwner", err)
			return
		}

		if c.IsTokenAuth && c.User.IsAdmin {
			c.Repo.AccessMode = models.ACCESS_MODE_OWNER
		} else {
			mode, err := models.AccessLevel(c.UserID(), repo)
			if err != nil {
				c.ServerError("AccessLevel", err)
				return
			}
			c.Repo.AccessMode = mode
		}

		if !c.Repo.HasAccess() {
			c.NotFound()
			return
		}

		c.Repo.Repository = repo
	}
}

// Contexter middleware already checks token for user sign in process.
func reqToken() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsTokenAuth {
			c.Error(http.StatusUnauthorized)
			return
		}
	}
}

func reqBasicAuth() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsBasicAuth {
			c.Error(http.StatusUnauthorized)
			return
		}
	}
}

func reqAdmin() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsLogged || !c.User.IsAdmin {
			c.Error(http.StatusForbidden)
			return
		}
	}
}

func reqRepoWriter() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsWriter() {
			c.Error(http.StatusForbidden)
			return
		}
	}
}

func reqRepoAdmin() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsAdmin() {
			c.Error(http.StatusForbidden)
			return
		}
	}
}

func orgAssignment(args ...bool) macaron.Handler {
	var (
		assignOrg  bool
		assignTeam bool
	)
	if len(args) > 0 {
		assignOrg = args[0]
	}
	if len(args) > 1 {
		assignTeam = args[1]
	}
	return func(c *context.APIContext) {
		c.Org = new(context.APIOrganization)

		var err error
		if assignOrg {
			c.Org.Organization, err = models.GetUserByName(c.Params(":orgname"))
			if err != nil {
				c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
				return
			}
		}

		if assignTeam {
			c.Org.Team, err = models.GetTeamByID(c.ParamsInt64(":teamid"))
			if err != nil {
				c.NotFoundOrServerError("GetTeamByID", errors.IsTeamNotExist, err)
				return
			}
		}
	}
}

func mustEnableIssues(c *context.APIContext) {
	if !c.Repo.Repository.EnableIssues || c.Repo.Repository.EnableExternalTracker {
		c.NotFound()
		return
	}
}

// RegisterRoutes registers all v1 APIs routes to web application.
// FIXME: custom form error response
func RegisterRoutes(m *macaron.Macaron) {
	bind := binding.Bind

	m.Group("/v1", func() {
		// Handle preflight OPTIONS request
		m.Options("/*", func() {})

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
				}, reqBasicAuth())
			})
		})

		m.Group("/users", func() {
			m.Group("/:username", func() {
				m.Get("/keys", user.ListPublicKeys)

				m.Get("/followers", user.ListFollowers)
				m.Group("/following", func() {
					m.Get("", user.ListFollowing)
					m.Get("/:target", user.CheckFollowing)
				})
			})
		}, reqToken())

		m.Group("/user", func() {
			m.Get("", user.GetAuthenticatedUser)
			m.Combo("/emails").Get(user.ListEmails).
				Post(bind(api.CreateEmailOption{}), user.AddEmail).
				Delete(bind(api.CreateEmailOption{}), user.DeleteEmail)

			m.Get("/followers", user.ListMyFollowers)
			m.Group("/following", func() {
				m.Get("", user.ListMyFollowing)
				m.Combo("/:username").Get(user.CheckMyFollowing).Put(user.Follow).Delete(user.Unfollow)
			})

			m.Group("/keys", func() {
				m.Combo("").Get(user.ListMyPublicKeys).
					Post(bind(api.CreateKeyOption{}), user.CreatePublicKey)
				m.Combo("/:id").Get(user.GetPublicKey).
					Delete(user.DeletePublicKey)
			})

			m.Combo("/issues").Get(repo.ListUserIssues)
		}, reqToken())

		// Repositories
		m.Get("/users/:username/repos", reqToken(), repo.ListUserRepositories)
		m.Get("/orgs/:org/repos", reqToken(), repo.ListOrgRepositories)
		m.Combo("/user/repos", reqToken()).Get(repo.ListMyRepos).
			Post(bind(api.CreateRepoOption{}), repo.Create)
		m.Post("/org/:org/repos", reqToken(), bind(api.CreateRepoOption{}), repo.CreateOrgRepo)

		m.Group("/repos", func() {
			m.Get("/search", repo.Search)

			m.Get("/:username/:reponame", repoAssignment(), repo.Get)
		})

		m.Group("/repos", func() {
			m.Post("/migrate", bind(form.MigrateRepo{}), repo.Migrate)
			m.Delete("/:username/:reponame", repoAssignment(), repo.Delete)

			m.Group("/:username/:reponame", func() {
				m.Group("/hooks", func() {
					m.Combo("").Get(repo.ListHooks).
						Post(bind(api.CreateHookOption{}), repo.CreateHook)
					m.Combo("/:id").Patch(bind(api.EditHookOption{}), repo.EditHook).
						Delete(repo.DeleteHook)
				}, reqRepoAdmin())
				m.Group("/collaborators", func() {
					m.Get("", repo.ListCollaborators)
					m.Combo("/:collaborator").Get(repo.IsCollaborator).Put(bind(api.AddCollaboratorOption{}), repo.AddCollaborator).
						Delete(repo.DeleteCollaborator)
				}, reqRepoAdmin())
				m.Get("/raw/*", context.RepoRef(), repo.GetRawFile)
				m.Get("/archive/*", repo.GetArchive)
				m.Get("/forks", repo.ListForks)
				m.Group("/branches", func() {
					m.Get("", repo.ListBranches)
					m.Get("/*", repo.GetBranch)
				})

				m.Group("/commits", func() {
					m.Get("/:sha", repo.GetSingleCommit)
					m.Get("/*", repo.GetReferenceSHA)
				})

				m.Group("/keys", func() {
					m.Combo("").Get(repo.ListDeployKeys).
						Post(bind(api.CreateKeyOption{}), repo.CreateDeployKey)
					m.Combo("/:id").Get(repo.GetDeployKey).
						Delete(repo.DeleteDeploykey)
				}, reqRepoAdmin())
				m.Group("/issues", func() {
					m.Combo("").Get(repo.ListIssues).Post(bind(api.CreateIssueOption{}), repo.CreateIssue)
					m.Group("/comments", func() {
						m.Get("", repo.ListRepoIssueComments)
						m.Combo("/:id").Patch(bind(api.EditIssueCommentOption{}), repo.EditIssueComment)
					})
					m.Group("/:index", func() {
						m.Combo("").Get(repo.GetIssue).Patch(bind(api.EditIssueOption{}), repo.EditIssue)

						m.Group("/comments", func() {
							m.Combo("").Get(repo.ListIssueComments).Post(bind(api.CreateIssueCommentOption{}), repo.CreateIssueComment)
							m.Combo("/:id").Patch(bind(api.EditIssueCommentOption{}), repo.EditIssueComment).
								Delete(repo.DeleteIssueComment)
						})

						m.Group("/labels", func() {
							m.Combo("").Get(repo.ListIssueLabels).
								Post(bind(api.IssueLabelsOption{}), repo.AddIssueLabels).
								Put(bind(api.IssueLabelsOption{}), repo.ReplaceIssueLabels).
								Delete(repo.ClearIssueLabels)
							m.Delete("/:id", repo.DeleteIssueLabel)
						})

					})
				}, mustEnableIssues)
				m.Group("/labels", func() {
					m.Combo("").Get(repo.ListLabels).
						Post(bind(api.CreateLabelOption{}), repo.CreateLabel)
					m.Combo("/:id").Get(repo.GetLabel).Patch(bind(api.EditLabelOption{}), repo.EditLabel).
						Delete(repo.DeleteLabel)
				})
				m.Group("/milestones", func() {
					m.Combo("").Get(repo.ListMilestones).
						Post(reqRepoWriter(), bind(api.CreateMilestoneOption{}), repo.CreateMilestone)
					m.Combo("/:id").Get(repo.GetMilestone).
						Patch(reqRepoWriter(), bind(api.EditMilestoneOption{}), repo.EditMilestone).
						Delete(reqRepoWriter(), repo.DeleteMilestone)
				})

				m.Patch("/issue-tracker", reqRepoWriter(), bind(api.EditIssueTrackerOption{}), repo.IssueTracker)
				m.Post("/mirror-sync", reqRepoWriter(), repo.MirrorSync)
				m.Get("/editorconfig/:filename", context.RepoRef(), repo.GetEditorconfig)
			}, repoAssignment())
		}, reqToken())

		m.Get("/issues", reqToken(), repo.ListUserIssues)

		// Organizations
		m.Combo("/user/orgs", reqToken()).Get(org.ListMyOrgs).Post(bind(api.CreateOrgOption{}), org.CreateMyOrg)

		m.Get("/users/:username/orgs", org.ListUserOrgs)
		m.Group("/orgs/:orgname", func() {
			m.Combo("").Get(org.Get).Patch(bind(api.EditOrgOption{}), org.Edit)
			m.Combo("/teams").Get(org.ListTeams)
		}, orgAssignment(true))

		m.Any("/*", func(c *context.Context) {
			c.NotFound()
		})

		m.Group("/admin", func() {
			m.Group("/users", func() {
				m.Post("", bind(api.CreateUserOption{}), admin.CreateUser)

				m.Group("/:username", func() {
					m.Combo("").Patch(bind(api.EditUserOption{}), admin.EditUser).
						Delete(admin.DeleteUser)
					m.Post("/keys", bind(api.CreateKeyOption{}), admin.CreatePublicKey)
					m.Post("/orgs", bind(api.CreateOrgOption{}), admin.CreateOrg)
					m.Post("/repos", bind(api.CreateRepoOption{}), admin.CreateRepo)
				})
			})

			m.Group("/orgs/:orgname", func() {
				m.Group("/teams", func() {
					m.Post("", orgAssignment(true), bind(api.CreateTeamOption{}), admin.CreateTeam)
				})
			})
			m.Group("/teams", func() {
				m.Group("/:teamid", func() {
					m.Combo("/members/:username").Put(admin.AddTeamMember).Delete(admin.RemoveTeamMember)
					m.Combo("/repos/:reponame").Put(admin.AddTeamRepository).Delete(admin.RemoveTeamRepository)
				}, orgAssignment(false, true))
			})
		}, reqAdmin())
	}, context.APIContexter())
}
