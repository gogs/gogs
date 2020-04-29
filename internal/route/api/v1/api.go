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

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/route/api/v1/admin"
	"gogs.io/gogs/internal/route/api/v1/misc"
	"gogs.io/gogs/internal/route/api/v1/org"
	"gogs.io/gogs/internal/route/api/v1/repo"
	"gogs.io/gogs/internal/route/api/v1/user"
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
				c.NotFoundOrError(err, "get user by name")
				return
			}
		}
		c.Repo.Owner = owner

		r, err := db.GetRepositoryByName(owner.ID, reponame)
		if err != nil {
			c.NotFoundOrError(err, "get repository by name")
			return
		} else if err = r.GetOwner(); err != nil {
			c.Error(err, "get owner")
			return
		}

		if c.IsTokenAuth && c.User.IsAdmin {
			c.Repo.AccessMode = db.AccessModeOwner
		} else {
			mode, err := db.UserAccessMode(c.UserID(), r)
			if err != nil {
				c.Error(err, "get user access mode")
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
				c.NotFoundOrError(err, "get organization by name")
				return
			}
		}

		if assignTeam {
			c.Org.Team, err = db.GetTeamByID(c.ParamsInt64(":teamid"))
			if err != nil {
				c.NotFoundOrError(err, "get team by ID")
				return
			}
		}
	}
}

// reqToken makes sure the context user is authorized via access token.
func reqToken() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsTokenAuth {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
}

// reqBasicAuth makes sure the context user is authorized via HTTP Basic Auth.
func reqBasicAuth() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsBasicAuth {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
}

// reqAdmin makes sure the context user is a site admin.
func reqAdmin() macaron.Handler {
	return func(c *context.Context) {
		if !c.IsLogged || !c.User.IsAdmin {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoWriter makes sure the context user has at least write access to the repository.
func reqRepoWriter() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsWriter() {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoWriter makes sure the context user has at least admin access to the repository.
func reqRepoAdmin() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsAdmin() {
			c.Status(http.StatusForbidden)
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
		m.Post("/markdown", bind(api.MarkdownOption{}), misc.Markdown)
		m.Post("/markdown/raw", misc.MarkdownRaw)

		// Users
		m.Group("/users", func() {
			m.Get("/search", user.Search)

			m.Group("/:username", func() {
				m.Get("", user.GetInfo)

				m.Group("/tokens", func() {
					m.Combo("").
						Get(user.ListAccessTokens).
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
			m.Combo("/emails").
				Get(user.ListEmails).
				Post(bind(api.CreateEmailOption{}), user.AddEmail).
				Delete(bind(api.CreateEmailOption{}), user.DeleteEmail)

			m.Get("/followers", user.ListMyFollowers)
			m.Group("/following", func() {
				m.Get("", user.ListMyFollowing)
				m.Combo("/:username").
					Get(user.CheckMyFollowing).
					Put(user.Follow).
					Delete(user.Unfollow)
			})

			m.Group("/keys", func() {
				m.Combo("").
					Get(user.ListMyPublicKeys).
					Post(bind(api.CreateKeyOption{}), user.CreatePublicKey)
				m.Combo("/:id").
					Get(user.GetPublicKey).
					Delete(user.DeletePublicKey)
			})

			m.Get("/issues", repo.ListUserIssues)
		}, reqToken())

		// Repositories
		m.Get("/users/:username/repos", reqToken(), repo.ListUserRepositories)
		m.Get("/orgs/:org/repos", reqToken(), repo.ListOrgRepositories)
		m.Combo("/user/repos", reqToken()).
			Get(repo.ListMyRepos).
			Post(bind(api.CreateRepoOption{}), repo.Create)
		m.Post("/org/:org/repos", reqToken(), bind(api.CreateRepoOption{}), repo.CreateOrgRepo)

		m.Group("/repos", func() {
			m.Get("/search", repo.Search)

			m.Get("/:username/:reponame", repoAssignment(), repo.Get)
			m.Get("/:username/:reponame/releases", repoAssignment(), repo.Releases)
		})

		m.Group("/repos", func() {
			m.Post("/migrate", bind(form.MigrateRepo{}), repo.Migrate)
			m.Delete("/:username/:reponame", repoAssignment(), repo.Delete)

			m.Group("/:username/:reponame", func() {
				m.Group("/hooks", func() {
					m.Combo("").
						Get(repo.ListHooks).
						Post(bind(api.CreateHookOption{}), repo.CreateHook)
					m.Combo("/:id").
						Patch(bind(api.EditHookOption{}), repo.EditHook).
						Delete(repo.DeleteHook)
				}, reqRepoAdmin())

				m.Group("/collaborators", func() {
					m.Get("", repo.ListCollaborators)
					m.Combo("/:collaborator").
						Get(repo.IsCollaborator).
						Put(bind(api.AddCollaboratorOption{}), repo.AddCollaborator).
						Delete(repo.DeleteCollaborator)
				}, reqRepoAdmin())

				m.Get("/raw/*", context.RepoRef(), repo.GetRawFile)
				m.Group("/contents", func() {
					m.Get("", repo.GetContents)
					m.Get("/*", repo.GetContents)
				})
				m.Get("/archive/*", repo.GetArchive)
				m.Group("/git/trees", func() {
					m.Get("/:sha", repo.GetRepoGitTree)
				})
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
					m.Combo("").
						Get(repo.ListDeployKeys).
						Post(bind(api.CreateKeyOption{}), repo.CreateDeployKey)
					m.Combo("/:id").
						Get(repo.GetDeployKey).
						Delete(repo.DeleteDeploykey)
				}, reqRepoAdmin())

				m.Group("/issues", func() {
					m.Combo("").
						Get(repo.ListIssues).
						Post(bind(api.CreateIssueOption{}), repo.CreateIssue)
					m.Group("/comments", func() {
						m.Get("", repo.ListRepoIssueComments)
						m.Patch("/:id", bind(api.EditIssueCommentOption{}), repo.EditIssueComment)
					})
					m.Group("/:index", func() {
						m.Combo("").
							Get(repo.GetIssue).
							Patch(bind(api.EditIssueOption{}), repo.EditIssue)

						m.Group("/comments", func() {
							m.Combo("").
								Get(repo.ListIssueComments).
								Post(bind(api.CreateIssueCommentOption{}), repo.CreateIssueComment)
							m.Combo("/:id").
								Patch(bind(api.EditIssueCommentOption{}), repo.EditIssueComment).
								Delete(repo.DeleteIssueComment)
						})

						m.Get("/labels", repo.ListIssueLabels)
						m.Group("/labels", func() {
							m.Combo("").
								Post(bind(api.IssueLabelsOption{}), repo.AddIssueLabels).
								Put(bind(api.IssueLabelsOption{}), repo.ReplaceIssueLabels).
								Delete(repo.ClearIssueLabels)
							m.Delete("/:id", repo.DeleteIssueLabel)
						}, reqRepoWriter())
					})
				}, mustEnableIssues)

				m.Group("/labels", func() {
					m.Get("", repo.ListLabels)
					m.Get("/:id", repo.GetLabel)
				})
				m.Group("/labels", func() {
					m.Post("", bind(api.CreateLabelOption{}), repo.CreateLabel)
					m.Combo("/:id").
						Patch(bind(api.EditLabelOption{}), repo.EditLabel).
						Delete(repo.DeleteLabel)
				}, reqRepoWriter())

				m.Group("/milestones", func() {
					m.Get("", repo.ListMilestones)
					m.Get("/:id", repo.GetMilestone)
				})
				m.Group("/milestones", func() {
					m.Post("", bind(api.CreateMilestoneOption{}), repo.CreateMilestone)
					m.Combo("/:id").
						Patch(bind(api.EditMilestoneOption{}), repo.EditMilestone).
						Delete(repo.DeleteMilestone)
				}, reqRepoWriter())

				m.Patch("/issue-tracker", reqRepoWriter(), bind(api.EditIssueTrackerOption{}), repo.IssueTracker)
				m.Post("/mirror-sync", reqRepoWriter(), repo.MirrorSync)
				m.Get("/editorconfig/:filename", context.RepoRef(), repo.GetEditorconfig)
			}, repoAssignment())
		}, reqToken())

		m.Get("/issues", reqToken(), repo.ListUserIssues)

		// Organizations
		m.Combo("/user/orgs", reqToken()).
			Get(org.ListMyOrgs).
			Post(bind(api.CreateOrgOption{}), org.CreateMyOrg)

		m.Get("/users/:username/orgs", org.ListUserOrgs)
		m.Group("/orgs/:orgname", func() {
			m.Combo("").
				Get(org.Get).
				Patch(bind(api.EditOrgOption{}), org.Edit)
			m.Get("/teams", org.ListTeams)
		}, orgAssignment(true))

		m.Group("/admin", func() {
			m.Group("/users", func() {
				m.Post("", bind(api.CreateUserOption{}), admin.CreateUser)

				m.Group("/:username", func() {
					m.Combo("").
						Patch(bind(api.EditUserOption{}), admin.EditUser).
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
					m.Get("/members", admin.ListTeamMembers)
					m.Combo("/members/:username").
						Put(admin.AddTeamMember).
						Delete(admin.RemoveTeamMember)
					m.Combo("/repos/:reponame").
						Put(admin.AddTeamRepository).
						Delete(admin.RemoveTeamRepository)
				}, orgAssignment(false, true))
			})
		}, reqAdmin())

		m.Any("/*", func(c *context.Context) {
			c.NotFound()
		})
	}, context.APIContexter())
}
