package v1

import (
	"net/http"
	"strings"

	"github.com/flamego/flamego"
	"github.com/go-macaron/binding"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/route/api/v1/admin"
	"gogs.io/gogs/internal/route/api/v1/misc"
	"gogs.io/gogs/internal/route/api/v1/org"
	"gogs.io/gogs/internal/route/api/v1/repo"
	"gogs.io/gogs/internal/route/api/v1/user"
)

// repoAssignment extracts information from URL parameters to retrieve the repository,
// and makes sure the context user has at least the read access to the repository.
func repoAssignment() flamego.Handler {
	return func(c *context.APIContext) {
		username := c.Param(":username")
		reponame := c.Param(":reponame")

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
func orgAssignment(args ...bool) flamego.Handler {
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
			c.Org.Organization, err = database.Handle.Users().GetByUsername(c.Req.Context(), c.Param(":orgname"))
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
func reqToken() flamego.Handler {
	return func(c *context.Context) {
		if !c.IsTokenAuth {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
}

// reqBasicAuth makes sure the context user is authorized via HTTP Basic Auth.
func reqBasicAuth() flamego.Handler {
	return func(c *context.Context) {
		if !c.IsBasicAuth {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
}

// reqAdmin makes sure the context user is a site admin.
func reqAdmin() flamego.Handler {
	return func(c *context.Context) {
		if !c.IsLogged || !c.User.IsAdmin {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoWriter makes sure the context user has at least write access to the repository.
func reqRepoWriter() flamego.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsWriter() {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoAdmin makes sure the context user has at least admin access to the repository.
func reqRepoAdmin() flamego.Handler {
	return func(c *context.Context) {
		if !c.Repo.IsAdmin() {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoOwner makes sure the context user has owner access to the repository.
func reqRepoOwner() flamego.Handler {
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
func RegisterRoutes(f flamego.Router) {
	bind := binding.Bind

	f.Group("/v1", func() {
		// Handle preflight OPTIONS request
		f.Options("/*", func() {})

		// Miscellaneous
		f.Post("/markdown", bind(api.MarkdownOption{}), misc.Markdown)
		f.Post("/markdown/raw", misc.MarkdownRaw)

		// Users
		f.Group("/users", func() {
			f.Get("/search", user.Search)

			f.Group("/:username", func() {
				f.Get("", user.GetInfo)

				f.Group("/tokens", func() {
					accessTokensHandler := user.NewAccessTokensHandler(user.NewAccessTokensStore())
					f.Combo("").
						Get(accessTokensHandler.List()).
						Post(bind(api.CreateAccessTokenOption{}), accessTokensHandler.Create())
				}, reqBasicAuth())
			})
		})

		f.Group("/users", func() {
			f.Group("/:username", func() {
				f.Get("/keys", user.ListPublicKeys)

				f.Get("/followers", user.ListFollowers)
				f.Group("/following", func() {
					f.Get("", user.ListFollowing)
					f.Get("/:target", user.CheckFollowing)
				})
			})
		}, reqToken())

		f.Group("/user", func() {
			f.Get("", user.GetAuthenticatedUser)
			f.Combo("/emails").
				Get(user.ListEmails).
				Post(bind(api.CreateEmailOption{}), user.AddEmail).
				Delete(bind(api.CreateEmailOption{}), user.DeleteEmail)

			f.Get("/followers", user.ListMyFollowers)
			f.Group("/following", func() {
				f.Get("", user.ListMyFollowing)
				f.Combo("/:username").
					Get(user.CheckMyFollowing).
					Put(user.Follow).
					Delete(user.Unfollow)
			})

			f.Group("/keys", func() {
				f.Combo("").
					Get(user.ListMyPublicKeys).
					Post(bind(api.CreateKeyOption{}), user.CreatePublicKey)
				f.Combo("/:id").
					Get(user.GetPublicKey).
					Delete(user.DeletePublicKey)
			})

			f.Get("/issues", repo.ListUserIssues)
		}, reqToken())

		// Repositories
		f.Get("/users/:username/repos", reqToken(), repo.ListUserRepositories)
		f.Get("/orgs/:org/repos", reqToken(), repo.ListOrgRepositories)
		f.Combo("/user/repos", reqToken()).
			Get(repo.ListMyRepos).
			Post(bind(api.CreateRepoOption{}), repo.Create)
		f.Post("/org/:org/repos", reqToken(), bind(api.CreateRepoOption{}), repo.CreateOrgRepo)

		f.Group("/repos", func() {
			f.Get("/search", repo.Search)

			f.Get("/:username/:reponame", repoAssignment(), repo.Get)
			f.Get("/:username/:reponame/releases", repoAssignment(), repo.Releases)
		})

		f.Group("/repos", func() {
			f.Post("/migrate", bind(form.MigrateRepo{}), repo.Migrate)
			f.Delete("/:username/:reponame", repoAssignment(), reqRepoOwner(), repo.Delete)

			f.Group("/:username/:reponame", func() {
				f.Group("/hooks", func() {
					f.Combo("").
						Get(repo.ListHooks).
						Post(bind(api.CreateHookOption{}), repo.CreateHook)
					f.Combo("/:id").
						Patch(bind(api.EditHookOption{}), repo.EditHook).
						Delete(repo.DeleteHook)
				}, reqRepoAdmin())

				f.Group("/collaborators", func() {
					f.Get("", repo.ListCollaborators)
					f.Combo("/:collaborator").
						Get(repo.IsCollaborator).
						Put(bind(api.AddCollaboratorOption{}), repo.AddCollaborator).
						Delete(repo.DeleteCollaborator)
				}, reqRepoAdmin())

				f.Get("/raw/*", context.RepoRef(), repo.GetRawFile)
				f.Group("/contents", func() {
					f.Get("", repo.GetContents)
					f.Combo("/*").
						Get(repo.GetContents).
						Put(reqRepoWriter(), bind(repo.PutContentsRequest{}), repo.PutContents)
				})
				f.Get("/archive/*", repo.GetArchive)
				f.Group("/git", func() {
					f.Group("/trees", func() {
						f.Get("/:sha", repo.GetRepoGitTree)
					})
					f.Group("/blobs", func() {
						f.Get("/:sha", repo.RepoGitBlob)
					})
				})
				f.Get("/forks", repo.ListForks)
				f.Get("/tags", repo.ListTags)
				f.Group("/branches", func() {
					f.Get("", repo.ListBranches)
					f.Get("/*", repo.GetBranch)
				})
				f.Group("/commits", func() {
					f.Get("/:sha", repo.GetSingleCommit)
					f.Get("", repo.GetAllCommits)
					f.Get("/*", repo.GetReferenceSHA)
				})

				f.Group("/keys", func() {
					f.Combo("").
						Get(repo.ListDeployKeys).
						Post(bind(api.CreateKeyOption{}), repo.CreateDeployKey)
					f.Combo("/:id").
						Get(repo.GetDeployKey).
						Delete(repo.DeleteDeploykey)
				}, reqRepoAdmin())

				f.Group("/issues", func() {
					f.Combo("").
						Get(repo.ListIssues).
						Post(bind(api.CreateIssueOption{}), repo.CreateIssue)
					f.Group("/comments", func() {
						f.Get("", repo.ListRepoIssueComments)
						f.Patch("/:id", bind(api.EditIssueCommentOption{}), repo.EditIssueComment)
					})
					f.Group("/:index", func() {
						f.Combo("").
							Get(repo.GetIssue).
							Patch(bind(api.EditIssueOption{}), repo.EditIssue)

						f.Group("/comments", func() {
							f.Combo("").
								Get(repo.ListIssueComments).
								Post(bind(api.CreateIssueCommentOption{}), repo.CreateIssueComment)
							f.Combo("/:id").
								Patch(bind(api.EditIssueCommentOption{}), repo.EditIssueComment).
								Delete(repo.DeleteIssueComment)
						})

						f.Get("/labels", repo.ListIssueLabels)
						f.Group("/labels", func() {
							f.Combo("").
								Post(bind(api.IssueLabelsOption{}), repo.AddIssueLabels).
								Put(bind(api.IssueLabelsOption{}), repo.ReplaceIssueLabels).
								Delete(repo.ClearIssueLabels)
							f.Delete("/:id", repo.DeleteIssueLabel)
						}, reqRepoWriter())
					})
				}, mustEnableIssues)

				f.Group("/labels", func() {
					f.Get("", repo.ListLabels)
					f.Get("/:id", repo.GetLabel)
				})
				f.Group("/labels", func() {
					f.Post("", bind(api.CreateLabelOption{}), repo.CreateLabel)
					f.Combo("/:id").
						Patch(bind(api.EditLabelOption{}), repo.EditLabel).
						Delete(repo.DeleteLabel)
				}, reqRepoWriter())

				f.Group("/milestones", func() {
					f.Get("", repo.ListMilestones)
					f.Get("/:id", repo.GetMilestone)
				})
				f.Group("/milestones", func() {
					f.Post("", bind(api.CreateMilestoneOption{}), repo.CreateMilestone)
					f.Combo("/:id").
						Patch(bind(api.EditMilestoneOption{}), repo.EditMilestone).
						Delete(repo.DeleteMilestone)
				}, reqRepoWriter())

				f.Patch("/issue-tracker", reqRepoWriter(), bind(api.EditIssueTrackerOption{}), repo.IssueTracker)
				f.Patch("/wiki", reqRepoWriter(), bind(api.EditWikiOption{}), repo.Wiki)
				f.Post("/mirror-sync", reqRepoWriter(), repo.MirrorSync)
				f.Get("/editorconfig/:filename", context.RepoRef(), repo.GetEditorconfig)
			}, repoAssignment())
		}, reqToken())

		f.Get("/issues", reqToken(), repo.ListUserIssues)

		// Organizations
		f.Combo("/user/orgs", reqToken()).
			Get(org.ListMyOrgs).
			Post(bind(api.CreateOrgOption{}), org.CreateMyOrg)

		f.Get("/users/:username/orgs", org.ListUserOrgs)
		f.Group("/orgs/:orgname", func() {
			f.Combo("").
				Get(org.Get).
				Patch(bind(api.EditOrgOption{}), org.Edit)
			f.Get("/teams", org.ListTeams)
		}, orgAssignment(true))

		f.Group("/admin", func() {
			f.Group("/users", func() {
				f.Post("", bind(api.CreateUserOption{}), admin.CreateUser)

				f.Group("/:username", func() {
					f.Combo("").
						Patch(bind(api.EditUserOption{}), admin.EditUser).
						Delete(admin.DeleteUser)
					f.Post("/keys", bind(api.CreateKeyOption{}), admin.CreatePublicKey)
					f.Post("/orgs", bind(api.CreateOrgOption{}), admin.CreateOrg)
					f.Post("/repos", bind(api.CreateRepoOption{}), admin.CreateRepo)
				})
			})

			f.Group("/orgs/:orgname", func() {
				f.Group("/teams", func() {
					f.Post("", orgAssignment(true), bind(api.CreateTeamOption{}), admin.CreateTeam)
				})
			})

			f.Group("/teams", func() {
				f.Group("/:teamid", func() {
					f.Get("/members", admin.ListTeamMembers)
					f.Combo("/members/:username").
						Put(admin.AddTeamMember).
						Delete(admin.RemoveTeamMember)
					f.Combo("/repos/:reponame").
						Put(admin.AddTeamRepository).
						Delete(admin.RemoveTeamRepository)
				}, orgAssignment(false, true))
			})
		}, reqAdmin())

		f.Any("/*", func(c *context.Context) {
			c.NotFound()
		})
	}, context.APIContexter())
}
