// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	admin2 "gogs.io/gogs/internal/route/api/v1/admin"
	misc2 "gogs.io/gogs/internal/route/api/v1/misc"
	org2 "gogs.io/gogs/internal/route/api/v1/org"
	repo2 "gogs.io/gogs/internal/route/api/v1/repo"
	user2 "gogs.io/gogs/internal/route/api/v1/user"
	"net/http"
	"strings"

	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/form"
)

// repoAssignment extracts information from URL parameters to retrieve the repository,
// and makes sure the context user has at least the read access to the repository.
func repoAssignment() macaron.Handler {
	return func(c *context.APIContext) {
		username := c.Params(":username")
		reponame := c.Params(":reponame")

		var err error
		var owner *db.User

		// Check if the context user is the repository owner.
		if c.IsLogged && c.User.LowerName == strings.ToLower(username) {
			owner = c.User
		} else {
			owner, err = db.GetUserByName(username)
			if err != nil {
				c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
				return
			}
		}
		c.Repo.Owner = owner

		r, err := db.GetRepositoryByName(owner.ID, reponame)
		if err != nil {
			c.NotFoundOrServerError("GetRepositoryByName", errors.IsRepoNotExist, err)
			return
		} else if err = r.GetOwner(); err != nil {
			c.ServerError("GetOwner", err)
			return
		}

		if c.IsTokenAuth && c.User.IsAdmin {
			c.Repo.AccessMode = db.ACCESS_MODE_OWNER
		} else {
			mode, err := db.UserAccessMode(c.UserID(), r)
			if err != nil {
				c.ServerError("UserAccessMode", err)
				return
			}
			c.Repo.AccessMode = mode
		}

		if !c.Repo.HasAccess() {
			c.NotFound()
			return
		}

		c.Repo.Repository = r
	}
}

// orgAssignment extracts information from URL parameters to retrieve the organization or team.
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
			c.Org.Organization, err = db.GetUserByName(c.Params(":orgname"))
			if err != nil {
				c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
				return
			}
		}

		if assignTeam {
			c.Org.Team, err = db.GetTeamByID(c.ParamsInt64(":teamid"))
			if err != nil {
				c.NotFoundOrServerError("GetTeamByID", errors.IsTeamNotExist, err)
				return
			}
		}
	}
}

// reqToken makes sure the context user is authorized via access token.
func reqToken() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsTokenAuth {
			c.Error(http.StatusUnauthorized)
			return
		}
	}
}

// reqBasicAuth makes sure the context user is authorized via HTTP Basic Auth.
func reqBasicAuth() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsBasicAuth {
			c.Error(http.StatusUnauthorized)
			return
		}
	}
}

// reqAdmin makes sure the context user is a site admin.
func reqAdmin() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsLogged || !c.User.IsAdmin {
			c.Error(http.StatusForbidden)
			return
		}
	}
}

// reqRepoWriter makes sure the context user has at least write access to the repository.
func reqRepoWriter() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsWriter() {
			c.Error(http.StatusForbidden)
			return
		}
	}
}

// reqRepoWriter makes sure the context user has at least admin access to the repository.
func reqRepoAdmin() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsAdmin() {
			c.Error(http.StatusForbidden)
			return
		}
	}
}

func mustEnableIssues(c *context.APIContext) {
	if !c.Repo.Repository.EnableIssues || c.Repo.Repository.EnableExternalTracker {
		c.NotFound()
		return
	}
}

// RegisterRoutes registers all route in API v1 to the web application.
// FIXME: custom form error response
func RegisterRoutes(m *macaron.Macaron) {
	bind := binding.Bind

	m.Group("/v1", func() {
		// Handle preflight OPTIONS request
		m.Options("/*", func() {})

		// Miscellaneous
		m.Post("/markdown", bind(api.MarkdownOption{}), misc2.Markdown)
		m.Post("/markdown/raw", misc2.MarkdownRaw)

		// Users
		m.Group("/users", func() {
			m.Get("/search", user2.Search)

			m.Group("/:username", func() {
				m.Get("", user2.GetInfo)

				m.Group("/tokens", func() {
					m.Combo("").
						Get(user2.ListAccessTokens).
						Post(bind(api.CreateAccessTokenOption{}), user2.CreateAccessToken)
				}, reqBasicAuth())
			})
		})

		m.Group("/users", func() {
			m.Group("/:username", func() {
				m.Get("/keys", user2.ListPublicKeys)

				m.Get("/followers", user2.ListFollowers)
				m.Group("/following", func() {
					m.Get("", user2.ListFollowing)
					m.Get("/:target", user2.CheckFollowing)
				})
			})
		}, reqToken())

		m.Group("/user", func() {
			m.Get("", user2.GetAuthenticatedUser)
			m.Combo("/emails").
				Get(user2.ListEmails).
				Post(bind(api.CreateEmailOption{}), user2.AddEmail).
				Delete(bind(api.CreateEmailOption{}), user2.DeleteEmail)

			m.Get("/followers", user2.ListMyFollowers)
			m.Group("/following", func() {
				m.Get("", user2.ListMyFollowing)
				m.Combo("/:username").
					Get(user2.CheckMyFollowing).
					Put(user2.Follow).
					Delete(user2.Unfollow)
			})

			m.Group("/keys", func() {
				m.Combo("").
					Get(user2.ListMyPublicKeys).
					Post(bind(api.CreateKeyOption{}), user2.CreatePublicKey)
				m.Combo("/:id").
					Get(user2.GetPublicKey).
					Delete(user2.DeletePublicKey)
			})

			m.Get("/issues", repo2.ListUserIssues)
		}, reqToken())

		// Repositories
		m.Get("/users/:username/repos", reqToken(), repo2.ListUserRepositories)
		m.Get("/orgs/:org/repos", reqToken(), repo2.ListOrgRepositories)
		m.Combo("/user/repos", reqToken()).
			Get(repo2.ListMyRepos).
			Post(bind(api.CreateRepoOption{}), repo2.Create)
		m.Post("/org/:org/repos", reqToken(), bind(api.CreateRepoOption{}), repo2.CreateOrgRepo)

		m.Group("/repos", func() {
			m.Get("/search", repo2.Search)

			m.Get("/:username/:reponame", repoAssignment(), repo2.Get)
		})

		m.Group("/repos", func() {
			m.Post("/migrate", bind(form.MigrateRepo{}), repo2.Migrate)
			m.Delete("/:username/:reponame", repoAssignment(), repo2.Delete)

			m.Group("/:username/:reponame", func() {
				m.Group("/hooks", func() {
					m.Combo("").
						Get(repo2.ListHooks).
						Post(bind(api.CreateHookOption{}), repo2.CreateHook)
					m.Combo("/:id").
						Patch(bind(api.EditHookOption{}), repo2.EditHook).
						Delete(repo2.DeleteHook)
				}, reqRepoAdmin())

				m.Group("/collaborators", func() {
					m.Get("", repo2.ListCollaborators)
					m.Combo("/:collaborator").
						Get(repo2.IsCollaborator).
						Put(bind(api.AddCollaboratorOption{}), repo2.AddCollaborator).
						Delete(repo2.DeleteCollaborator)
				}, reqRepoAdmin())

				m.Get("/raw/*", context.RepoRef(), repo2.GetRawFile)
				m.Get("/archive/*", repo2.GetArchive)
				m.Group("/git/trees", func() {
					m.Get("/:sha", context.RepoRef(), repo2.GetRepoGitTree)
				})
				m.Get("/forks", repo2.ListForks)
				m.Group("/branches", func() {
					m.Get("", repo2.ListBranches)
					m.Get("/*", repo2.GetBranch)
				})
				m.Group("/commits", func() {
					m.Get("/:sha", repo2.GetSingleCommit)
					m.Get("/*", repo2.GetReferenceSHA)
				})

				m.Group("/keys", func() {
					m.Combo("").
						Get(repo2.ListDeployKeys).
						Post(bind(api.CreateKeyOption{}), repo2.CreateDeployKey)
					m.Combo("/:id").
						Get(repo2.GetDeployKey).
						Delete(repo2.DeleteDeploykey)
				}, reqRepoAdmin())

				m.Group("/issues", func() {
					m.Combo("").
						Get(repo2.ListIssues).
						Post(bind(api.CreateIssueOption{}), repo2.CreateIssue)
					m.Group("/comments", func() {
						m.Get("", repo2.ListRepoIssueComments)
						m.Patch("/:id", bind(api.EditIssueCommentOption{}), repo2.EditIssueComment)
					})
					m.Group("/:index", func() {
						m.Combo("").
							Get(repo2.GetIssue).
							Patch(bind(api.EditIssueOption{}), repo2.EditIssue)

						m.Group("/comments", func() {
							m.Combo("").
								Get(repo2.ListIssueComments).
								Post(bind(api.CreateIssueCommentOption{}), repo2.CreateIssueComment)
							m.Combo("/:id").
								Patch(bind(api.EditIssueCommentOption{}), repo2.EditIssueComment).
								Delete(repo2.DeleteIssueComment)
						})

						m.Get("/labels", repo2.ListIssueLabels)
						m.Group("/labels", func() {
							m.Combo("").
								Post(bind(api.IssueLabelsOption{}), repo2.AddIssueLabels).
								Put(bind(api.IssueLabelsOption{}), repo2.ReplaceIssueLabels).
								Delete(repo2.ClearIssueLabels)
							m.Delete("/:id", repo2.DeleteIssueLabel)
						}, reqRepoWriter())
					})
				}, mustEnableIssues)

				m.Group("/labels", func() {
					m.Get("", repo2.ListLabels)
					m.Get("/:id", repo2.GetLabel)
				})
				m.Group("/labels", func() {
					m.Post("", bind(api.CreateLabelOption{}), repo2.CreateLabel)
					m.Combo("/:id").
						Patch(bind(api.EditLabelOption{}), repo2.EditLabel).
						Delete(repo2.DeleteLabel)
				}, reqRepoWriter())

				m.Group("/milestones", func() {
					m.Get("", repo2.ListMilestones)
					m.Get("/:id", repo2.GetMilestone)
				})
				m.Group("/milestones", func() {
					m.Post("", bind(api.CreateMilestoneOption{}), repo2.CreateMilestone)
					m.Combo("/:id").
						Patch(bind(api.EditMilestoneOption{}), repo2.EditMilestone).
						Delete(repo2.DeleteMilestone)
				}, reqRepoWriter())

				m.Patch("/issue-tracker", reqRepoWriter(), bind(api.EditIssueTrackerOption{}), repo2.IssueTracker)
				m.Post("/mirror-sync", reqRepoWriter(), repo2.MirrorSync)
				m.Get("/editorconfig/:filename", context.RepoRef(), repo2.GetEditorconfig)
			}, repoAssignment())
		}, reqToken())

		m.Get("/issues", reqToken(), repo2.ListUserIssues)

		// Organizations
		m.Combo("/user/orgs", reqToken()).
			Get(org2.ListMyOrgs).
			Post(bind(api.CreateOrgOption{}), org2.CreateMyOrg)

		m.Get("/users/:username/orgs", org2.ListUserOrgs)
		m.Group("/orgs/:orgname", func() {
			m.Combo("").
				Get(org2.Get).
				Patch(bind(api.EditOrgOption{}), org2.Edit)
			m.Get("/teams", org2.ListTeams)
		}, orgAssignment(true))

		m.Group("/admin", func() {
			m.Group("/users", func() {
				m.Post("", bind(api.CreateUserOption{}), admin2.CreateUser)

				m.Group("/:username", func() {
					m.Combo("").
						Patch(bind(api.EditUserOption{}), admin2.EditUser).
						Delete(admin2.DeleteUser)
					m.Post("/keys", bind(api.CreateKeyOption{}), admin2.CreatePublicKey)
					m.Post("/orgs", bind(api.CreateOrgOption{}), admin2.CreateOrg)
					m.Post("/repos", bind(api.CreateRepoOption{}), admin2.CreateRepo)
				})
			})

			m.Group("/orgs/:orgname", func() {
				m.Group("/teams", func() {
					m.Post("", orgAssignment(true), bind(api.CreateTeamOption{}), admin2.CreateTeam)
				})
			})

			m.Group("/teams", func() {
				m.Group("/:teamid", func() {
					m.Combo("/members/:username").
						Put(admin2.AddTeamMember).
						Delete(admin2.RemoveTeamMember)
					m.Combo("/repos/:reponame").
						Put(admin2.AddTeamRepository).
						Delete(admin2.RemoveTeamRepository)
				}, orgAssignment(false, true))
			})
		}, reqAdmin())

		m.Any("/*", func(c *context.Context) {
			c.NotFound()
		})
	}, context.APIContexter())
}
