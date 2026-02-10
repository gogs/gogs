package v1

import (
	"net/http"
	"strings"

	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
)

// repoAssignment extracts information from URL parameters to retrieve the repository,
// and makes sure the context user has at least the read access to the repository.
func repoAssignment() macaron.Handler {
	return func(c *context.APIContext) {
		username := c.Params(":username")
		reponame := c.Params(":reponame")

		var err error
		var owner *database.User

		// Check if the context user is the repository owner.
		if c.IsLogged && c.User.LowerName == strings.ToLower(username) {
			owner = c.User
		} else {
			owner, err = database.Handle.Users().GetByUsername(c.Req.Context(), username)
			if err != nil {
				c.NotFoundOrError(err, "get user by name")
				return
			}
		}
		c.Repo.Owner = owner

		repo, err := database.Handle.Repositories().GetByName(c.Req.Context(), owner.ID, reponame)
		if err != nil {
			c.NotFoundOrError(err, "get repository by name")
			return
		} else if err = repo.GetOwner(); err != nil {
			c.Error(err, "get owner")
			return
		}

		if c.IsTokenAuth && c.User.IsAdmin {
			c.Repo.AccessMode = database.AccessModeOwner
		} else {
			c.Repo.AccessMode = database.Handle.Permissions().AccessMode(c.Req.Context(), c.UserID(), repo.ID,
				database.AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
		}

		if !c.Repo.HasAccess() {
			c.NotFound()
			return
		}

		c.Repo.Repository = repo
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
			c.Org.Organization, err = database.Handle.Users().GetByUsername(c.Req.Context(), c.Params(":orgname"))
			if err != nil {
				c.NotFoundOrError(err, "get organization by name")
				return
			}
		}

		if assignTeam {
			c.Org.Team, err = database.GetTeamByID(c.ParamsInt64(":teamid"))
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

// reqRepoAdmin makes sure the context user has at least admin access to the repository.
func reqRepoAdmin() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsAdmin() {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoOwner makes sure the context user has owner access to the repository.
func reqRepoOwner() macaron.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsOwner() {
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
		m.Post("/markdown", bind(MarkdownRequest{}), Markdown)
		m.Post("/markdown/raw", MarkdownRaw)

		// Users
		m.Group("/users", func() {
			m.Get("/search", SearchUsers)

			m.Group("/:username", func() {
				m.Get("", GetInfo)

				m.Group("/tokens", func() {
					accessTokensHandler := NewAccessTokensHandler(NewAccessTokensStore())
					m.Combo("").
						Get(accessTokensHandler.List()).
						Post(bind(CreateAccessTokenRequest{}), accessTokensHandler.Create())
				}, reqBasicAuth())
			})
		})

		m.Group("/users", func() {
			m.Group("/:username", func() {
				m.Get("/keys", ListPublicKeys)

				m.Get("/followers", ListFollowers)
				m.Group("/following", func() {
					m.Get("", ListFollowing)
					m.Get("/:target", CheckFollowing)
				})
			})
		}, reqToken())

		m.Group("/user", func() {
			m.Get("", GetAuthenticatedUser)
			m.Combo("/emails").
				Get(ListEmails).
				Post(bind(CreateEmailRequest{}), AddEmail).
				Delete(bind(CreateEmailRequest{}), DeleteEmail)

			m.Get("/followers", ListMyFollowers)
			m.Group("/following", func() {
				m.Get("", ListMyFollowing)
				m.Combo("/:username").
					Get(CheckMyFollowing).
					Put(Follow).
					Delete(Unfollow)
			})

			m.Group("/keys", func() {
				m.Combo("").
					Get(ListMyPublicKeys).
					Post(bind(CreatePublicKeyRequest{}), CreatePublicKey)
				m.Combo("/:id").
					Get(GetPublicKey).
					Delete(DeletePublicKey)
			})

			m.Get("/issues", ListUserIssues)
		}, reqToken())

		// Repositories
		m.Get("/users/:username/repos", reqToken(), ListUserRepositories)
		m.Get("/orgs/:org/repos", reqToken(), ListOrgRepositories)
		m.Combo("/user/repos", reqToken()).
			Get(ListMyRepos).
			Post(bind(CreateRepoRequest{}), Create)
		m.Post("/org/:org/repos", reqToken(), bind(CreateRepoRequest{}), CreateOrgRepo)

		m.Group("/repos", func() {
			m.Get("/search", SearchRepos)

			m.Get("/:username/:reponame", repoAssignment(), GetRepo)
			m.Get("/:username/:reponame/releases", repoAssignment(), Releases)
		})

		m.Group("/repos", func() {
			m.Post("/migrate", bind(form.MigrateRepo{}), Migrate)
			m.Delete("/:username/:reponame", repoAssignment(), reqRepoOwner(), Delete)

			m.Group("/:username/:reponame", func() {
				m.Group("/hooks", func() {
					m.Combo("").
						Get(ListHooks).
						Post(bind(CreateHookRequest{}), CreateHook)
					m.Combo("/:id").
						Patch(bind(EditHookRequest{}), EditHook).
						Delete(DeleteHook)
				}, reqRepoAdmin())

				m.Group("/collaborators", func() {
					m.Get("", ListCollaborators)
					m.Combo("/:collaborator").
						Get(IsCollaborator).
						Put(bind(AddCollaboratorRequest{}), AddCollaborator).
						Delete(DeleteCollaborator)
				}, reqRepoAdmin())

				m.Get("/raw/*", context.RepoRef(), GetRawFile)
				m.Group("/contents", func() {
					m.Get("", GetContents)
					m.Combo("/*").
						Get(GetContents).
						Put(reqRepoWriter(), bind(PutContentsRequest{}), PutContents)
				})
				m.Get("/archive/*", GetArchive)
				m.Group("/git", func() {
					m.Group("/trees", func() {
						m.Get("/:sha", GetRepoGitTree)
					})
					m.Group("/blobs", func() {
						m.Get("/:sha", RepoGitBlob)
					})
				})
				m.Get("/forks", ListForks)
				m.Get("/tags", ListTags)
				m.Group("/branches", func() {
					m.Get("", ListBranches)
					m.Get("/*", GetBranch)
				})
				m.Group("/commits", func() {
					m.Get("/:sha", GetSingleCommit)
					m.Get("", GetAllCommits)
					m.Get("/*", GetReferenceSHA)
				})

				m.Group("/keys", func() {
					m.Combo("").
						Get(ListDeployKeys).
						Post(bind(CreateDeployKeyRequest{}), CreateDeployKey)
					m.Combo("/:id").
						Get(GetDeployKey).
						Delete(DeleteDeploykey)
				}, reqRepoAdmin())

				m.Group("/issues", func() {
					m.Combo("").
						Get(ListIssues).
						Post(bind(CreateIssueRequest{}), CreateIssue)
					m.Group("/comments", func() {
						m.Get("", ListRepoIssueComments)
						m.Patch("/:id", bind(EditIssueCommentRequest{}), EditIssueComment)
					})
					m.Group("/:index", func() {
						m.Combo("").
							Get(GetIssue).
							Patch(bind(EditIssueRequest{}), EditIssue)

						m.Group("/comments", func() {
							m.Combo("").
								Get(ListIssueComments).
								Post(bind(CreateIssueCommentRequest{}), CreateIssueComment)
							m.Combo("/:id").
								Patch(bind(EditIssueCommentRequest{}), EditIssueComment).
								Delete(DeleteIssueComment)
						})

						m.Get("/labels", ListIssueLabels)
						m.Group("/labels", func() {
							m.Combo("").
								Post(bind(IssueLabelsRequest{}), AddIssueLabels).
								Put(bind(IssueLabelsRequest{}), ReplaceIssueLabels).
								Delete(ClearIssueLabels)
							m.Delete("/:id", DeleteIssueLabel)
						}, reqRepoWriter())
					})
				}, mustEnableIssues)

				m.Group("/labels", func() {
					m.Get("", ListLabels)
					m.Get("/:id", GetLabel)
				})
				m.Group("/labels", func() {
					m.Post("", bind(CreateLabelRequest{}), CreateLabel)
					m.Combo("/:id").
						Patch(bind(EditLabelRequest{}), EditLabel).
						Delete(DeleteLabel)
				}, reqRepoWriter())

				m.Group("/milestones", func() {
					m.Get("", ListMilestones)
					m.Get("/:id", GetMilestone)
				})
				m.Group("/milestones", func() {
					m.Post("", bind(CreateMilestoneRequest{}), CreateMilestone)
					m.Combo("/:id").
						Patch(bind(EditMilestoneRequest{}), EditMilestone).
						Delete(DeleteMilestone)
				}, reqRepoWriter())

				m.Patch("/issue-tracker", reqRepoWriter(), bind(EditIssueTrackerRequest{}), IssueTracker)
				m.Patch("/wiki", reqRepoWriter(), bind(EditWikiRequest{}), Wiki)
				m.Post("/mirror-sync", reqRepoWriter(), MirrorSync)
				m.Get("/editorconfig/:filename", context.RepoRef(), GetEditorconfig)
			}, repoAssignment())
		}, reqToken())

		m.Get("/issues", reqToken(), ListUserIssues)

		// Organizations
		m.Combo("/user/orgs", reqToken()).
			Get(ListMyOrgs).
			Post(bind(CreateOrgRequest{}), CreateMyOrg)

		m.Get("/users/:username/orgs", ListUserOrgs)
		m.Group("/orgs/:orgname", func() {
			m.Combo("").
				Get(GetOrg).
				Patch(bind(EditOrgRequest{}), EditOrg)
			m.Get("/teams", ListTeams)
		}, orgAssignment(true))

		m.Group("/admin", func() {
			m.Group("/users", func() {
				m.Post("", bind(CreateUserRequest{}), AdminCreateUser)

				m.Group("/:username", func() {
					m.Combo("").
						Patch(bind(EditUserRequest{}), AdminEditUser).
						Delete(AdminDeleteUser)
					m.Post("/keys", bind(CreatePublicKeyRequest{}), AdminCreatePublicKey)
					m.Post("/orgs", bind(CreateOrgRequest{}), AdminCreateOrg)
					m.Post("/repos", bind(CreateRepoRequest{}), AdminCreateRepo)
				})
			})

			m.Group("/orgs/:orgname", func() {
				m.Group("/teams", func() {
					m.Post("", orgAssignment(true), bind(CreateTeamRequest{}), AdminCreateTeam)
				})
			})

			m.Group("/teams", func() {
				m.Group("/:teamid", func() {
					m.Get("/members", AdminListTeamMembers)
					m.Combo("/members/:username").
						Put(AdminAddTeamMember).
						Delete(AdminRemoveTeamMember)
					m.Combo("/repos/:reponame").
						Put(AdminAddTeamRepository).
						Delete(AdminRemoveTeamRepository)
				}, orgAssignment(false, true))
			})
		}, reqAdmin())

		m.Any("/*", func(c *context.Context) {
			c.NotFound()
		})
	}, context.APIContexter())
}
