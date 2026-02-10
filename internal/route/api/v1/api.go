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
		m.Post("/markdown", bind(markdownRequest{}), markdown)
		m.Post("/markdown/raw", markdownRaw)

		// Users
		m.Group("/users", func() {
			m.Get("/search", searchUsers)

			m.Group("/:username", func() {
				m.Get("", getUserProfile)

				m.Group("/tokens", func() {
					accessTokensHandler := newAccessTokensHandler(newAccessTokensStore())
					m.Combo("").
						Get(accessTokensHandler.List()).
						Post(bind(createAccessTokenRequest{}), accessTokensHandler.Create())
				}, reqBasicAuth())
			})
		})

		m.Group("/users", func() {
			m.Group("/:username", func() {
				m.Get("/keys", listPublicKeys)

				m.Get("/followers", listFollowers)
				m.Group("/following", func() {
					m.Get("", listFollowing)
					m.Get("/:target", checkFollowing)
				})
			})
		}, reqToken())

		m.Group("/user", func() {
			m.Get("", getAuthenticatedUser)
			m.Combo("/emails").
				Get(listEmails).
				Post(bind(createEmailRequest{}), addEmail).
				Delete(bind(createEmailRequest{}), deleteEmail)

			m.Get("/followers", listMyFollowers)
			m.Group("/following", func() {
				m.Get("", listMyFollowing)
				m.Combo("/:username").
					Get(checkMyFollowing).
					Put(follow).
					Delete(unfollow)
			})

			m.Group("/keys", func() {
				m.Combo("").
					Get(listMyPublicKeys).
					Post(bind(createPublicKeyRequest{}), createPublicKey)
				m.Combo("/:id").
					Get(getPublicKey).
					Delete(deletePublicKey)
			})

			m.Get("/issues", listUserIssues)
		}, reqToken())

		// Repositories
		m.Get("/users/:username/repos", reqToken(), listUserRepositories)
		m.Get("/orgs/:org/repos", reqToken(), listOrgRepositories)
		m.Combo("/user/repos", reqToken()).
			Get(listMyRepos).
			Post(bind(createRepoRequest{}), createRepo)
		m.Post("/org/:org/repos", reqToken(), bind(createRepoRequest{}), createOrgRepo)

		m.Group("/repos", func() {
			m.Get("/search", searchRepos)

			m.Get("/:username/:reponame", repoAssignment(), getRepo)
			m.Get("/:username/:reponame/releases", repoAssignment(), releases)
		})

		m.Group("/repos", func() {
			m.Post("/migrate", bind(form.MigrateRepo{}), migrate)
			m.Delete("/:username/:reponame", repoAssignment(), reqRepoOwner(), deleteRepo)

			m.Group("/:username/:reponame", func() {
				m.Group("/hooks", func() {
					m.Combo("").
						Get(listHooks).
						Post(bind(createHookRequest{}), createHook)
					m.Combo("/:id").
						Patch(bind(editHookRequest{}), editHook).
						Delete(deleteHook)
				}, reqRepoAdmin())

				m.Group("/collaborators", func() {
					m.Get("", listCollaborators)
					m.Combo("/:collaborator").
						Get(isCollaborator).
						Put(bind(addCollaboratorRequest{}), addCollaborator).
						Delete(deleteCollaborator)
				}, reqRepoAdmin())

				m.Get("/raw/*", context.RepoRef(), getRawFile)
				m.Group("/contents", func() {
					m.Get("", getContents)
					m.Combo("/*").
						Get(getContents).
						Put(reqRepoWriter(), bind(putContentsRequest{}), putContents)
				})
				m.Get("/archive/*", getArchive)
				m.Group("/git", func() {
					m.Group("/trees", func() {
						m.Get("/:sha", getRepoGitTree)
					})
					m.Group("/blobs", func() {
						m.Get("/:sha", repoGitBlob)
					})
				})
				m.Get("/forks", listForks)
				m.Get("/tags", listTags)
				m.Group("/branches", func() {
					m.Get("", listBranches)
					m.Get("/*", getBranch)
				})
				m.Group("/commits", func() {
					m.Get("/:sha", getSingleCommit)
					m.Get("", getAllCommits)
					m.Get("/*", getReferenceSHA)
				})

				m.Group("/keys", func() {
					m.Combo("").
						Get(listDeployKeys).
						Post(bind(createDeployKeyRequest{}), createDeployKey)
					m.Combo("/:id").
						Get(getDeployKey).
						Delete(deleteDeploykey)
				}, reqRepoAdmin())

				m.Group("/issues", func() {
					m.Combo("").
						Get(listIssues).
						Post(bind(createIssueRequest{}), createIssue)
					m.Group("/comments", func() {
						m.Get("", listRepoIssueComments)
						m.Patch("/:id", bind(editIssueCommentRequest{}), editIssueComment)
					})
					m.Group("/:index", func() {
						m.Combo("").
							Get(getIssue).
							Patch(bind(editIssueRequest{}), editIssue)

						m.Group("/comments", func() {
							m.Combo("").
								Get(listIssueComments).
								Post(bind(createIssueCommentRequest{}), createIssueComment)
							m.Combo("/:id").
								Patch(bind(editIssueCommentRequest{}), editIssueComment).
								Delete(deleteIssueComment)
						})

						m.Get("/labels", listIssueLabels)
						m.Group("/labels", func() {
							m.Combo("").
								Post(bind(issueLabelsRequest{}), addIssueLabels).
								Put(bind(issueLabelsRequest{}), replaceIssueLabels).
								Delete(clearIssueLabels)
							m.Delete("/:id", deleteIssueLabel)
						}, reqRepoWriter())
					})
				}, mustEnableIssues)

				m.Group("/labels", func() {
					m.Get("", listLabels)
					m.Get("/:id", getLabel)
				})
				m.Group("/labels", func() {
					m.Post("", bind(createLabelRequest{}), createLabel)
					m.Combo("/:id").
						Patch(bind(editLabelRequest{}), editLabel).
						Delete(deleteLabel)
				}, reqRepoWriter())

				m.Group("/milestones", func() {
					m.Get("", listMilestones)
					m.Get("/:id", getMilestone)
				})
				m.Group("/milestones", func() {
					m.Post("", bind(createMilestoneRequest{}), createMilestone)
					m.Combo("/:id").
						Patch(bind(editMilestoneRequest{}), editMilestone).
						Delete(deleteMilestone)
				}, reqRepoWriter())

				m.Patch("/issue-tracker", reqRepoWriter(), bind(editIssueTrackerRequest{}), issueTracker)
				m.Patch("/wiki", reqRepoWriter(), bind(editWikiRequest{}), wiki)
				m.Post("/mirror-sync", reqRepoWriter(), mirrorSync)
				m.Get("/editorconfig/:filename", context.RepoRef(), getEditorconfig)
			}, repoAssignment())
		}, reqToken())

		m.Get("/issues", reqToken(), listUserIssues)

		// Organizations
		m.Combo("/user/orgs", reqToken()).
			Get(listMyOrgs).
			Post(bind(createOrgRequest{}), createMyOrg)

		m.Get("/users/:username/orgs", listUserOrgs)
		m.Group("/orgs/:orgname", func() {
			m.Combo("").
				Get(getOrg).
				Patch(bind(editOrgRequest{}), editOrg)
			m.Get("/teams", listTeams)
		}, orgAssignment(true))

		m.Group("/admin", func() {
			m.Group("/users", func() {
				m.Post("", bind(adminCreateUserRequest{}), adminCreateUser)

				m.Group("/:username", func() {
					m.Combo("").
						Patch(bind(adminEditUserRequest{}), adminEditUser).
						Delete(adminDeleteUser)
					m.Post("/keys", bind(createPublicKeyRequest{}), adminCreatePublicKey)
					m.Post("/orgs", bind(createOrgRequest{}), adminCreateOrg)
					m.Post("/repos", bind(createRepoRequest{}), adminCreateRepo)
				})
			})

			m.Group("/orgs/:orgname", func() {
				m.Group("/teams", func() {
					m.Post("", orgAssignment(true), bind(adminCreateTeamRequest{}), adminCreateTeam)
				})
			})

			m.Group("/teams", func() {
				m.Group("/:teamid", func() {
					m.Get("/members", adminListTeamMembers)
					m.Combo("/members/:username").
						Put(adminAddTeamMember).
						Delete(adminRemoveTeamMember)
					m.Combo("/repos/:reponame").
						Put(adminAddTeamRepository).
						Delete(adminRemoveTeamRepository)
				}, orgAssignment(false, true))
			})
		}, reqAdmin())

		m.Any("/*", func(c *context.Context) {
			c.NotFound()
		})
	}, context.APIContexter())
}
