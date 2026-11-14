package v1

import (
	"net/http"
	"strings"

	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/database"
)

// repoAssignment extracts information from URL parameters to retrieve the repository,
// and makes sure the context user has at least the read access to the repository.
func repoAssignment() flamego.Handler {
	return func(c *apiContext) {
		username := c.Param("username")
		reponame := c.Param("reponame")

		var err error
		var owner *database.User

		// Check if the context user is the repository owner.
		if c.IsLogged && c.User.LowerName == strings.ToLower(username) {
			owner = c.User
		} else {
			owner, err = database.Handle.Users().GetByUsername(c.Request().Context(), username)
			if err != nil {
				c.NotFoundOrError(err, "get user by name")
				return
			}
		}
		c.Repo.Owner = owner

		repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), owner.ID, reponame)
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
			c.Repo.AccessMode = database.Handle.Permissions().AccessMode(c.Request().Context(), c.UserID(), repo.ID,
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
	return func(c *apiContext) {
		c.Org = new(apiOrganization)

		var err error
		if assignOrg {
			c.Org.Organization, err = database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("orgname"))
			if err != nil {
				c.NotFoundOrError(err, "get organization by name")
				return
			}
		}

		if assignTeam {
			c.Org.Team, err = database.GetTeamByID(c.ParamInt64("teamid"))
			if err != nil {
				c.NotFoundOrError(err, "get team by ID")
				return
			}
		}
	}
}

// reqToken makes sure the context user is authorized via access token.
func reqToken() flamego.Handler {
	return func(c *apiContext) {
		if !c.IsTokenAuth {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
}

// reqBasicAuth makes sure the context user is authorized via HTTP Basic Auth.
func reqBasicAuth() flamego.Handler {
	return func(c *apiContext) {
		if !c.IsBasicAuth {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
}

// reqAdmin makes sure the context user is a site admin.
func reqAdmin() flamego.Handler {
	return func(c *apiContext) {
		if !c.IsLogged || !c.User.IsAdmin {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoWriter makes sure the context user has at least write access to the repository.
func reqRepoWriter() flamego.Handler {
	return func(c *apiContext) {
		if !c.Repo.IsWriter() {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoAdmin makes sure the context user has at least admin access to the repository.
func reqRepoAdmin() flamego.Handler {
	return func(c *apiContext) {
		if !c.Repo.IsAdmin() {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

// reqRepoOwner makes sure the context user has owner access to the repository.
func reqRepoOwner() flamego.Handler {
	return func(c *apiContext) {
		if !c.Repo.IsOwner() {
			c.Status(http.StatusForbidden)
			return
		}
	}
}

func mustEnableIssues(c *apiContext) {
	if !c.Repo.Repository.EnableIssues || c.Repo.Repository.EnableExternalTracker {
		c.NotFound()
		return
	}
}

// RegisterRoutes registers all routes of API v1 under "/api/v1".
func RegisterRoutes(f *flamego.Flame) {
	// Serve HEAD requests with GET handlers as Macaron did, only for the routes
	// registered below.
	f.AutoHead(true)
	defer f.AutoHead(false)

	f.Group("/api/v1", func() {
		// Handle preflight OPTIONS request
		f.Options("/{**}", func() {})

		// Miscellaneous
		f.Post("/markdown", bind(markdownRequest{}), markdown)
		f.Post("/markdown/raw", markdownRaw)

		// Users
		f.Group("/users", func() {
			f.Get("/search", searchUsers)

			f.Group("/{username}", func() {
				f.Get("", getUserProfile)

				f.Group("/tokens", func() {
					accessTokensHandler := newAccessTokensHandler(newAccessTokensStore())
					f.Combo("").
						Get(accessTokensHandler.List()).
						Post(bind(createAccessTokenRequest{}), accessTokensHandler.Create())
				}, reqBasicAuth())
			})
		})

		f.Group("/users", func() {
			f.Group("/{username}", func() {
				f.Get("/keys", listPublicKeys)

				f.Get("/followers", listFollowers)
				f.Group("/following", func() {
					f.Get("", listFollowing)
					f.Get("/{target}", checkFollowing)
				})
			})
		}, reqToken())

		f.Group("/user", func() {
			f.Get("", getAuthenticatedUser)
			f.Combo("/emails").
				Get(listEmails).
				Post(bind(createEmailRequest{}), addEmail).
				Delete(bind(createEmailRequest{}), deleteEmail)

			f.Get("/followers", listMyFollowers)
			f.Group("/following", func() {
				f.Get("", listMyFollowing)
				f.Combo("/{username}").
					Get(checkMyFollowing).
					Put(follow).
					Delete(unfollow)
			})

			f.Group("/keys", func() {
				f.Combo("").
					Get(listMyPublicKeys).
					Post(bind(createPublicKeyRequest{}), createPublicKey)
				f.Combo("/{id}").
					Get(getPublicKey).
					Delete(deletePublicKey)
			})

			f.Get("/issues", listUserIssues)
		}, reqToken())

		// Repositories
		f.Get("/users/{username}/repos", reqToken(), listUserRepositories)
		f.Get("/orgs/{org}/repos", reqToken(), listOrgRepositories)
		f.Combo("/user/repos", reqToken()).
			Get(listMyRepos).
			Post(bind(createRepoRequest{}), createRepo)
		f.Post("/org/{org}/repos", reqToken(), bind(createRepoRequest{}), createOrgRepo)

		f.Group("/repos", func() {
			f.Get("/search", searchRepos)

			f.Get("/{username}/{reponame}", repoAssignment(), getRepo)
			f.Get("/{username}/{reponame}/releases", repoAssignment(), releases)
		})

		f.Group("/repos", func() {
			f.Post("/migrate", bind(migrateRepoRequest{}), migrate)
			f.Delete("/{username}/{reponame}", repoAssignment(), reqRepoOwner(), deleteRepo)

			f.Group("/{username}/{reponame}", func() {
				f.Group("/hooks", func() {
					f.Combo("").
						Get(listHooks).
						Post(bind(createHookRequest{}), createHook)
					f.Combo("/{id}").
						Patch(bind(editHookRequest{}), editHook).
						Delete(deleteHook)
				}, reqRepoAdmin())

				f.Group("/collaborators", func() {
					f.Get("", listCollaborators)
					f.Combo("/{collaborator}").
						Get(isCollaborator).
						Put(bind(addCollaboratorRequest{}), addCollaborator).
						Delete(deleteCollaborator)
				}, reqRepoAdmin())

				f.Get("/raw/{**}", getRawFile)
				f.Group("/contents", func() {
					f.Get("", getContents)
					f.Combo("/{**}").
						Get(getContents).
						Put(reqRepoWriter(), bind(putContentsRequest{}), putContents)
				})
				f.Get("/archive/{**}", getArchive)
				f.Group("/git", func() {
					f.Group("/trees", func() {
						f.Get("/{sha}", getRepoGitTree)
					})
					f.Group("/blobs", func() {
						f.Get("/{sha}", repoGitBlob)
					})
				})
				f.Get("/forks", listForks)
				f.Get("/tags", listTags)
				f.Group("/branches", func() {
					f.Get("", listBranches)
					f.Get("/{**}", getBranch)
				})
				f.Group("/commits", func() {
					f.Get("/{sha}", getSingleCommit)
					f.Get("", getAllCommits)
					f.Get("/{**}", getReferenceSHA)
				})

				f.Group("/keys", func() {
					f.Combo("").
						Get(listDeployKeys).
						Post(bind(createDeployKeyRequest{}), createDeployKey)
					f.Combo("/{id}").
						Get(getDeployKey).
						Delete(deleteDeploykey)
				}, reqRepoAdmin())

				f.Group("/issues", func() {
					f.Combo("").
						Get(listIssues).
						Post(bind(createIssueRequest{}), createIssue)
					f.Group("/comments", func() {
						f.Get("", listRepoIssueComments)
						f.Patch("/{id}", bind(editIssueCommentRequest{}), editIssueComment)
					})
					f.Group("/{index}", func() {
						f.Combo("").
							Get(getIssue).
							Patch(bind(editIssueRequest{}), editIssue)

						f.Group("/comments", func() {
							f.Combo("").
								Get(listIssueComments).
								Post(bind(createIssueCommentRequest{}), createIssueComment)
							f.Combo("/{id}").
								Patch(bind(editIssueCommentRequest{}), editIssueComment).
								Delete(deleteIssueComment)
						})

						f.Get("/labels", listIssueLabels)
						f.Group("/labels", func() {
							f.Combo("").
								Post(bind(issueLabelsRequest{}), addIssueLabels).
								Put(bind(issueLabelsRequest{}), replaceIssueLabels).
								Delete(clearIssueLabels)
							f.Delete("/{id}", deleteIssueLabel)
						}, reqRepoWriter())
					})
				}, mustEnableIssues)

				f.Group("/labels", func() {
					f.Get("", listLabels)
					f.Get("/{id}", getLabel)
				})
				f.Group("/labels", func() {
					f.Post("", bind(createLabelRequest{}), createLabel)
					f.Combo("/{id}").
						Patch(bind(editLabelRequest{}), editLabel).
						Delete(deleteLabel)
				}, reqRepoWriter())

				f.Group("/milestones", func() {
					f.Get("", listMilestones)
					f.Get("/{id}", getMilestone)
				})
				f.Group("/milestones", func() {
					f.Post("", bind(createMilestoneRequest{}), createMilestone)
					f.Combo("/{id}").
						Patch(bind(editMilestoneRequest{}), editMilestone).
						Delete(deleteMilestone)
				}, reqRepoWriter())

				f.Patch("/issue-tracker", reqRepoAdmin(), bind(editIssueTrackerRequest{}), issueTracker)
				f.Patch("/wiki", reqRepoAdmin(), bind(editWikiRequest{}), wiki)
				f.Post("/mirror-sync", reqRepoAdmin(), mirrorSync)
				f.Get("/editorconfig/{filename}", getEditorconfig)
			}, repoAssignment())
		}, reqToken())

		f.Get("/issues", reqToken(), listUserIssues)

		// Organizations
		f.Combo("/user/orgs", reqToken()).
			Get(listMyOrgs).
			Post(bind(createOrgRequest{}), createMyOrg)

		f.Get("/users/{username}/orgs", listUserOrgs)
		f.Group("/orgs/{orgname}", func() {
			f.Combo("").
				Get(getOrg).
				Patch(bind(editOrgRequest{}), editOrg)
			f.Get("/teams", listTeams)
		}, reqToken(), orgAssignment(true))

		f.Group("/admin", func() {
			f.Group("/users", func() {
				f.Post("", bind(adminCreateUserRequest{}), adminCreateUser)

				f.Group("/{username}", func() {
					f.Combo("").
						Patch(bind(adminEditUserRequest{}), adminEditUser).
						Delete(adminDeleteUser)
					f.Post("/keys", bind(createPublicKeyRequest{}), adminCreatePublicKey)
					f.Post("/orgs", bind(createOrgRequest{}), adminCreateOrg)
					f.Post("/repos", bind(createRepoRequest{}), adminCreateRepo)
				})
			})

			f.Group("/orgs/{orgname}", func() {
				f.Group("/teams", func() {
					f.Post("", orgAssignment(true), bind(adminCreateTeamRequest{}), adminCreateTeam)
				})
			})

			f.Group("/teams", func() {
				f.Group("/{teamid}", func() {
					f.Get("/members", adminListTeamMembers)
					f.Combo("/members/{username}").
						Put(adminAddTeamMember).
						Delete(adminRemoveTeamMember)
					f.Combo("/repos/{reponame}").
						Put(adminAddTeamRepository).
						Delete(adminRemoveTeamRepository)
				}, orgAssignment(false, true))
			})
		}, reqAdmin())

		// Preflight OPTIONS requests are handled above.
		f.Routes("/{**}", "GET,HEAD,POST,PUT,PATCH,DELETE", func(c *apiContext) {
			c.NotFound()
		})
	}, apiContexter)
}
