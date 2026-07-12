package web

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/captcha"
	"github.com/flamego/flamego"
	"github.com/go-macaron/binding"
	macaroncache "github.com/go-macaron/cache"
	"github.com/go-macaron/gzip"
	"github.com/go-macaron/i18n"
	"github.com/go-macaron/session"
	"github.com/gogs/git-module"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	embedconf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/app"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/route"
	"gogs.io/gogs/internal/route/admin"
	apiv1 "gogs.io/gogs/internal/route/api/v1"
	"gogs.io/gogs/internal/route/lfs"
	"gogs.io/gogs/internal/route/org"
	"gogs.io/gogs/internal/route/repo"
	"gogs.io/gogs/internal/route/user"
	"gogs.io/gogs/internal/ssh"
	"gogs.io/gogs/internal/template"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/urlx"
	"gogs.io/gogs/public"
	"gogs.io/gogs/templates"
)

// Run starts the web server with the given configuration path and port override.
func Run(configPath string, portOverride int) error {
	err := initServices(configPath)
	if err != nil {
		return errors.Wrap(err, "initialize application")
	}

	m, err := newMacaron()
	if err != nil {
		return errors.Wrap(err, "initialize macaron")
	}

	webHandler, err := newRoutingHandler()
	if err != nil {
		return errors.Wrap(err, "initialize web handler")
	}

	reqSignIn := context.Toggle(&context.ToggleOptions{SignInRequired: true})
	ignSignIn := context.Toggle(&context.ToggleOptions{SignInRequired: conf.Auth.RequireSigninView})

	bindIgnErr := binding.BindIgnErr

	m.SetAutoHead(true)

	m.Group("", func() {
		m.Get("/", ignSignIn, route.Home)
		m.Group("/explore", func() {
			m.Get("", func(c *context.Context) {
				c.Redirect(conf.Server.Subpath + "/explore/repos")
			})
			m.Get("/repos", route.ExploreRepos)
			m.Get("/users", route.ExploreUsers)
			m.Get("/organizations", route.ExploreOrganizations)
		}, ignSignIn)
		m.Get("/^:type(issues|pulls)$", reqSignIn, user.Issues)

		// ***** START: User *****
		m.Group("/user/settings", func() {
			m.Get("", user.Settings)
			m.Post("", bindIgnErr(form.UpdateProfile{}), user.SettingsPost)
			m.Combo("/avatar").Get(user.SettingsAvatar).
				Post(binding.MultipartForm(form.Avatar{}), user.SettingsAvatarPost)
			m.Post("/avatar/delete", user.SettingsDeleteAvatar)
			m.Combo("/email").Get(user.SettingsEmails).
				Post(bindIgnErr(form.AddEmail{}), user.SettingsEmailPost)
			m.Post("/email/delete", user.DeleteEmail)
			m.Get("/password", user.SettingsPassword)
			m.Post("/password", bindIgnErr(form.ChangePassword{}), user.SettingsPasswordPost)
			m.Combo("/ssh").Get(user.SettingsSSHKeys).
				Post(bindIgnErr(form.AddSSHKey{}), user.SettingsSSHKeysPost)
			m.Post("/ssh/delete", user.DeleteSSHKey)
			m.Group("/security", func() {
				m.Get("", user.SettingsSecurity)
				m.Combo("/two_factor_enable").Get(user.SettingsTwoFactorEnable).
					Post(user.SettingsTwoFactorEnablePost)
				m.Combo("/two_factor_recovery_codes").Get(user.SettingsTwoFactorRecoveryCodes).
					Post(user.SettingsTwoFactorRecoveryCodesPost)
				m.Post("/two_factor_disable", user.SettingsTwoFactorDisable)
			})
			m.Group("/repositories", func() {
				m.Get("", user.SettingsRepos)
				m.Post("/leave", user.SettingsLeaveRepo)
			})
			m.Group("/organizations", func() {
				m.Get("", user.SettingsOrganizations)
				m.Post("/leave", user.SettingsLeaveOrganization)
			})

			settingsHandler := user.NewSettingsHandler(user.NewSettingsStore())
			m.Combo("/applications").Get(settingsHandler.Applications()).
				Post(bindIgnErr(form.NewAccessToken{}), settingsHandler.ApplicationsPost())
			m.Post("/applications/delete", settingsHandler.DeleteApplication())
			m.Route("/delete", "GET,POST", user.SettingsDelete)
		}, reqSignIn, func(c *context.Context) {
			c.Data["PageIsUserSettings"] = true
		})

		m.Group("/user", func() {
			m.Any("/activate_email", user.ActivateEmail)
			m.Get("/email2user", user.Email2User)
		})
		// ***** END: User *****

		reqAdmin := context.Toggle(&context.ToggleOptions{SignInRequired: true, AdminRequired: true})

		// ***** START: Admin *****
		m.Group("/admin", func() {
			m.Combo("").Get(admin.Dashboard).Post(admin.Operation) // "/admin"
			m.Get("/config", admin.Config)
			m.Post("/config/test_mail", admin.SendTestMail)
			m.Get("/monitor", admin.Monitor)

			m.Group("/users", func() {
				m.Get("", admin.Users)
				m.Combo("/new").Get(admin.NewUser).Post(bindIgnErr(form.AdminCrateUser{}), admin.NewUserPost)
				m.Combo("/:userid").Get(admin.EditUser).Post(bindIgnErr(form.AdminEditUser{}), admin.EditUserPost)
				m.Post("/:userid/delete", admin.DeleteUser)
			})

			m.Group("/orgs", func() {
				m.Get("", admin.Organizations)
			})

			m.Group("/repos", func() {
				m.Get("", admin.Repos)
				m.Post("/delete", admin.DeleteRepo)
			})

			m.Group("/auths", func() {
				m.Get("", admin.Authentications)
				m.Combo("/new").Get(admin.NewAuthSource).Post(bindIgnErr(form.Authentication{}), admin.NewAuthSourcePost)
				m.Combo("/:authid").Get(admin.EditAuthSource).
					Post(bindIgnErr(form.Authentication{}), admin.EditAuthSourcePost)
				m.Post("/:authid/delete", admin.DeleteAuthSource)
			})

			m.Group("/notices", func() {
				m.Get("", admin.Notices)
				m.Post("/delete", admin.DeleteNotices)
				m.Get("/empty", admin.EmptyNotices)
			})
		}, reqAdmin)
		// ***** END: Admin *****

		m.Group("", func() {
			m.Group("/:username", func() {
				m.Get("", user.Profile)
				m.Get("/followers", user.Followers)
				m.Get("/following", user.Following)
				m.Get("/stars", user.Stars)
			}, context.InjectParamsUser())

			m.Get("/attachments/:uuid", func(c *context.Context) {
				attach, err := database.GetAttachmentByUUID(c.Params(":uuid"))
				if err != nil {
					c.NotFoundOrError(err, "get attachment by UUID")
					return
				}

				// Resolve the repository that owns this attachment so we can enforce
				// repository-level read permission. Without this check, anyone with
				// the UUID could download files belonging to private repositories.
				var repo *database.Repository
				switch {
				case attach.IssueID > 0:
					issue, err := database.GetIssueByID(attach.IssueID)
					if err != nil {
						c.NotFoundOrError(err, "get issue by ID")
						return
					}
					repo = issue.Repo
				case attach.ReleaseID > 0:
					release, err := database.GetReleaseByID(attach.ReleaseID)
					if err != nil {
						c.NotFoundOrError(err, "get release by ID")
						return
					}
					repo = release.Repo
				}
				if repo == nil {
					c.NotFound()
					return
				}
				if !repo.HasAccess(c.UserID()) {
					c.NotFound()
					return
				}

				if !osx.IsFile(attach.LocalPath()) {
					c.NotFound()
					return
				}

				fr, err := os.Open(attach.LocalPath())
				if err != nil {
					c.Error(err, "open attachment file")
					return
				}
				defer fr.Close()

				c.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
				c.Header().Set("Cache-Control", "private,max-age=86400")
				c.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, attach.Name))

				if _, err = io.Copy(c.Resp, fr); err != nil {
					c.Error(err, "copy from file to response")
					return
				}
			})
		}, ignSignIn)

		m.Group("", func() {
			m.Post("/issues/attachments", repo.UploadIssueAttachment)
			m.Post("/releases/attachments", repo.UploadReleaseAttachment)
		}, reqSignIn)

		m.Group("/:username", func() {
			m.Post("/action/:action", user.Action)
		}, reqSignIn, context.InjectParamsUser())

		reqRepoAdmin := context.RequireRepoAdmin()
		reqRepoWriter := context.RequireRepoWriter()

		webhookRoutes := func() {
			m.Group("", func() {
				m.Get("", repo.Webhooks)
				m.Post("/delete", repo.DeleteWebhook)
				m.Get("/:type/new", repo.WebhooksNew)
				m.Post("/gogs/new", bindIgnErr(form.NewWebhook{}), repo.WebhooksNewPost)
				m.Post("/slack/new", bindIgnErr(form.NewSlackHook{}), repo.WebhooksSlackNewPost)
				m.Post("/discord/new", bindIgnErr(form.NewDiscordHook{}), repo.WebhooksDiscordNewPost)
				m.Post("/dingtalk/new", bindIgnErr(form.NewDingtalkHook{}), repo.WebhooksDingtalkNewPost)
				m.Get("/:id", repo.WebhooksEdit)
				m.Post("/gogs/:id", bindIgnErr(form.NewWebhook{}), repo.WebhooksEditPost)
				m.Post("/slack/:id", bindIgnErr(form.NewSlackHook{}), repo.WebhooksSlackEditPost)
				m.Post("/discord/:id", bindIgnErr(form.NewDiscordHook{}), repo.WebhooksDiscordEditPost)
				m.Post("/dingtalk/:id", bindIgnErr(form.NewDingtalkHook{}), repo.WebhooksDingtalkEditPost)
			}, repo.InjectOrgRepoContext())
		}

		// ***** START: Organization *****
		m.Group("/org", func() {
			m.Group("", func() {
				m.Get("/create", org.Create)
				m.Post("/create", bindIgnErr(form.CreateOrg{}), org.CreatePost)
			}, func(c *context.Context) {
				if !c.User.CanCreateOrganization() {
					c.NotFound()
				}
			})

			m.Group("/:org", func() {
				m.Get("/dashboard", user.Dashboard)
				m.Get("/^:type(issues|pulls)$", user.Issues)
				m.Get("/members", org.Members)
				m.Post("/members/action/:action", org.MembersAction)

				m.Get("/teams", org.Teams)
			}, context.OrgAssignment(true))

			m.Group("/:org", func() {
				m.Get("/teams/:team", org.TeamMembers)
				m.Get("/teams/:team/repositories", org.TeamRepositories)
				m.Post("/teams/:team/action/:action", org.TeamsAction)
				m.Post("/teams/:team/action/repo/:action", org.TeamsRepoAction)
			}, context.OrgAssignment(true, false, true))

			m.Group("/:org", func() {
				m.Get("/teams/new", org.NewTeam)
				m.Post("/teams/new", bindIgnErr(form.CreateTeam{}), org.NewTeamPost)
				m.Get("/teams/:team/edit", org.EditTeam)
				m.Post("/teams/:team/edit", bindIgnErr(form.CreateTeam{}), org.EditTeamPost)
				m.Post("/teams/:team/delete", org.DeleteTeam)

				m.Group("/settings", func() {
					m.Combo("").Get(org.Settings).
						Post(bindIgnErr(form.UpdateOrgSetting{}), org.SettingsPost)
					m.Post("/avatar", binding.MultipartForm(form.Avatar{}), org.SettingsAvatar)
					m.Post("/avatar/delete", org.SettingsDeleteAvatar)
					m.Group("/hooks", webhookRoutes)
					m.Route("/delete", "GET,POST", org.SettingsDelete)
				})

				m.Route("/invitations/new", "GET,POST", org.Invitation)
			}, context.OrgAssignment(true, true))
		}, reqSignIn)
		// ***** END: Organization *****

		// ***** START: Repository *****
		m.Group("/repo", func() {
			m.Get("/create", repo.Create)
			m.Post("/create", bindIgnErr(form.CreateRepo{}), repo.CreatePost)
			m.Get("/migrate", repo.Migrate)
			m.Post("/migrate", bindIgnErr(form.MigrateRepo{}), repo.MigratePost)
			m.Combo("/fork/:repoid").Get(repo.Fork).
				Post(bindIgnErr(form.CreateRepo{}), repo.ForkPost)
		}, reqSignIn)

		m.Group("/:username/:reponame", func() {
			m.Group("/settings", func() {
				m.Combo("").Get(repo.Settings).
					Post(bindIgnErr(form.RepoSetting{}), repo.SettingsPost)
				m.Combo("/avatar").Get(repo.SettingsAvatar).
					Post(binding.MultipartForm(form.Avatar{}), repo.SettingsAvatarPost)
				m.Post("/avatar/delete", repo.SettingsDeleteAvatar)
				m.Group("/collaboration", func() {
					m.Combo("").Get(repo.SettingsCollaboration).Post(repo.SettingsCollaborationPost)
					m.Post("/access_mode", repo.ChangeCollaborationAccessMode)
					m.Post("/delete", repo.DeleteCollaboration)
				})
				m.Group("/branches", func() {
					m.Get("", repo.SettingsBranches)
					m.Post("/default_branch", repo.UpdateDefaultBranch)
					m.Combo("/*").Get(repo.SettingsProtectedBranch).
						Post(bindIgnErr(form.ProtectBranch{}), repo.SettingsProtectedBranchPost)
				}, func(c *context.Context) {
					if c.Repo.Repository.IsMirror {
						c.NotFound()
						return
					}
				})

				m.Group("/hooks", func() {
					webhookRoutes()

					m.Group("/:id", func() {
						m.Post("/test", repo.TestWebhook)
						m.Post("/redelivery", repo.RedeliveryWebhook)
					})

					m.Group("/git", func() {
						m.Get("", repo.SettingsGitHooks)
						m.Combo("/:name").Get(repo.SettingsGitHooksEdit).
							Post(repo.SettingsGitHooksEditPost)
					}, context.GitHookService())
				})

				m.Group("/keys", func() {
					m.Combo("").Get(repo.SettingsDeployKeys).
						Post(bindIgnErr(form.AddSSHKey{}), repo.SettingsDeployKeysPost)
					m.Post("/delete", repo.DeleteDeployKey)
				})
			}, func(c *context.Context) {
				c.Data["PageIsSettings"] = true
			})
		}, reqSignIn, context.RepoAssignment(), reqRepoAdmin, context.RepoRef())

		m.Post("/:username/:reponame/action/:action", reqSignIn, context.RepoAssignment(), repo.Action)
		m.Group("/:username/:reponame", func() {
			m.Get("/issues", repo.RetrieveLabels, repo.Issues)
			m.Get("/issues/:index", repo.ViewIssue)
			m.Get("/labels/", repo.RetrieveLabels, repo.Labels)
			m.Get("/milestones", repo.Milestones)
		}, ignSignIn, context.RepoAssignment(true))
		m.Group("/:username/:reponame", func() {
			// FIXME: should use different URLs but mostly same logic for comments of issue and pull request.
			// So they can apply their own enable/disable logic on routers.
			m.Group("/issues", func() {
				m.Combo("/new", repo.MustEnableIssues).Get(context.RepoRef(), repo.NewIssue).
					Post(bindIgnErr(form.NewIssue{}), repo.NewIssuePost)

				m.Group("/:index", func() {
					m.Post("/title", repo.UpdateIssueTitle)
					m.Post("/content", repo.UpdateIssueContent)
					m.Combo("/comments").Post(bindIgnErr(form.CreateComment{}), repo.NewComment)
				})
			})
			m.Group("/comments/:id", func() {
				m.Post("", repo.UpdateCommentContent)
				m.Post("/delete", repo.DeleteComment)
			})
		}, reqSignIn, context.RepoAssignment(true))
		m.Group("/:username/:reponame", func() {
			m.Group("/wiki", func() {
				m.Get("/?:page", repo.Wiki)
				m.Get("/_pages", repo.WikiPages)
			}, repo.MustEnableWiki, context.RepoRef())
		}, ignSignIn, context.RepoAssignment(false, true))

		m.Group("/:username/:reponame", func() {
			// FIXME: should use different URLs but mostly same logic for comments of issue and pull request.
			// So they can apply their own enable/disable logic on routers.
			m.Group("/issues", func() {
				m.Group("/:index", func() {
					m.Post("/label", repo.UpdateIssueLabel)
					m.Post("/milestone", repo.UpdateIssueMilestone)
					m.Post("/assignee", repo.UpdateIssueAssignee)
				}, reqRepoWriter)
			})
			m.Group("/labels", func() {
				m.Post("/new", bindIgnErr(form.CreateLabel{}), repo.NewLabel)
				m.Post("/edit", bindIgnErr(form.CreateLabel{}), repo.UpdateLabel)
				m.Post("/delete", repo.DeleteLabel)
				m.Post("/initialize", bindIgnErr(form.InitializeLabels{}), repo.InitializeLabels)
			}, reqRepoWriter, context.RepoRef())
			m.Group("/milestones", func() {
				m.Combo("/new").Get(repo.NewMilestone).
					Post(bindIgnErr(form.CreateMilestone{}), repo.NewMilestonePost)
				m.Get("/:id/edit", repo.EditMilestone)
				m.Post("/:id/edit", bindIgnErr(form.CreateMilestone{}), repo.EditMilestonePost)
				m.Get("/:id/:action", repo.ChangeMilestonStatus)
				m.Post("/delete", repo.DeleteMilestone)
			}, reqRepoWriter, context.RepoRef())

			m.Group("/releases", func() {
				m.Get("/new", repo.NewRelease)
				m.Post("/new", bindIgnErr(form.NewRelease{}), repo.NewReleasePost)
				m.Post("/delete", repo.DeleteRelease)
				m.Get("/edit/*", repo.EditRelease)
				m.Post("/edit/*", bindIgnErr(form.EditRelease{}), repo.EditReleasePost)
			}, repo.MustBeNotBare, reqRepoWriter, func(c *context.Context) {
				c.Data["PageIsViewFiles"] = true
			})

			// FIXME: Should use c.Repo.PullRequest to unify the template. Same-repo PR URLs include a
			// redundant head user, e.g. /org1/test-repo/compare/master...org1:develop should be
			// /org1/test-repo/compare/master...develop.
			m.Combo("/compare/*", repo.MustAllowPulls).Get(repo.CompareAndPullRequest).
				Post(bindIgnErr(form.NewIssue{}), repo.CompareAndPullRequestPost)

			m.Group("", func() {
				m.Combo("/_edit/*").Get(repo.EditFile).
					Post(bindIgnErr(form.EditRepoFile{}), repo.EditFilePost)
				m.Combo("/_new/*").Get(repo.NewFile).
					Post(bindIgnErr(form.EditRepoFile{}), repo.NewFilePost)
				m.Post("/_preview/*", bindIgnErr(form.EditPreviewDiff{}), repo.DiffPreviewPost)
				m.Combo("/_delete/*").Get(repo.DeleteFile).
					Post(bindIgnErr(form.DeleteRepoFile{}), repo.DeleteFilePost)

				m.Group("", func() {
					m.Combo("/_upload/*").Get(repo.UploadFile).
						Post(bindIgnErr(form.UploadRepoFile{}), repo.UploadFilePost)
					m.Post("/upload-file", repo.UploadFileToServer)
					m.Post("/upload-remove", bindIgnErr(form.RemoveUploadFile{}), repo.RemoveUploadFileFromServer)
				}, func(c *context.Context) {
					if !conf.Repository.Upload.Enabled {
						c.NotFound()
						return
					}
				})
			}, repo.MustBeNotBare, reqRepoWriter, context.RepoRef(), func(c *context.Context) {
				if !c.Repo.CanEnableEditor() {
					c.NotFound()
					return
				}

				c.Data["PageIsViewFiles"] = true
			})
		}, reqSignIn, context.RepoAssignment())

		m.Group("/:username/:reponame", func() {
			m.Group("", func() {
				m.Get("/releases", repo.MustBeNotBare, repo.Releases)
				m.Get("/pulls", repo.RetrieveLabels, repo.Pulls)
				m.Get("/pulls/:index", repo.ViewPull)
			}, context.RepoRef())

			m.Group("/branches", func() {
				m.Get("", repo.Branches)
				m.Get("/all", repo.AllBranches)
				m.Post("/delete/*", reqSignIn, reqRepoWriter, repo.DeleteBranchPost)
			}, repo.MustBeNotBare, func(c *context.Context) {
				c.Data["PageIsViewFiles"] = true
			})

			m.Group("/wiki", func() {
				m.Group("", func() {
					m.Combo("/_new").Get(repo.NewWiki).
						Post(bindIgnErr(form.NewWiki{}), repo.NewWikiPost)
					m.Combo("/:page/_edit").Get(repo.EditWiki).
						Post(bindIgnErr(form.NewWiki{}), repo.EditWikiPost)
					m.Post("/:page/delete", repo.DeleteWikiPagePost)
				}, reqSignIn, reqRepoWriter)
			}, repo.MustEnableWiki, context.RepoRef())

			m.Get("/archive/*", repo.MustBeNotBare, repo.Download)

			m.Group("/pulls/:index", func() {
				m.Get("/commits", context.RepoRef(), repo.ViewPullCommits)
				m.Get("/files", context.RepoRef(), repo.ViewPullFiles)
				m.Post("/merge", reqRepoWriter, repo.MergePullRequest)
			}, repo.MustAllowPulls)

			m.Group("", func() {
				m.Get("/src/*", repo.Home)
				m.Get("/commits/*", repo.RefCommits)
				m.Get("/forks", repo.Forks)
			}, repo.MustBeNotBare, context.RepoRef())
			// Bridged to Flamego to skip the legacy `RepoRef` middleware, which double-resolves the ref.
			m.Get("/raw/*", flamegoBridger(webHandler))
			m.Get("/commit/:sha([a-f0-9]{7,40})\\.:ext(patch|diff)", flamegoBridger(webHandler))
			// Constrain SHA shape so non-matching `/commit/...` paths 404 instead of loading the SPA with a bad param.
			m.Get("/commit/:sha([a-f0-9]{7,40})$", repo.MustBeNotBare, func(c *context.Context) { c.ServeWeb() })

			m.Get("/compare/:before([a-z0-9]{40})\\.\\.\\.:after([a-z0-9]{40})", repo.MustBeNotBare, context.RepoRef(), repo.CompareDiff)
		}, ignSignIn, context.RepoAssignment())
		m.Group("/:username/:reponame", func() {
			m.Get("", repo.Home)
			m.Get("/stars", repo.Stars)
			m.Get("/watchers", repo.Watchers)
		}, context.ServeGoGet(), ignSignIn, context.RepoAssignment(), context.RepoRef())
		// ***** END: Repository *****

		// **********************
		// ----- API routes -----
		// **********************

		// TODO: Without session
		m.Group("/api", func() {
			apiv1.RegisterRoutes(m)
		}, ignSignIn)

		m.Any("/api/web/*", flamegoBridger(webHandler))
		m.Get("/redirect", flamegoBridger(webHandler))
		m.Get("/captcha/*", flamegoBridger(webHandler))
		m.Any("/*", func(c *context.Context) { c.ServeWeb() })
	},
		session.Sessioner(session.Options{
			Provider:       conf.Session.Provider,
			ProviderConfig: conf.Session.ProviderConfig,
			CookieName:     conf.Session.CookieName,
			CookiePath:     conf.Server.Subpath,
			Gclifetime:     conf.Session.GCInterval,
			Maxlifetime:    conf.Session.MaxLifeTime,
			Secure:         conf.Session.CookieSecure,
			CookieLifeTime: 86400 * conf.Security.LoginRememberDays,
		}),
		context.Contexter(context.NewStore(), webHandler),
	)

	// ***************************
	// ----- HTTP Git routes -----
	// ***************************

	m.Group("/:username/:reponame", func() {
		m.Get("/tasks/trigger", repo.TriggerTask)

		m.Group("/info/lfs", func() {
			lfs.RegisterRoutes(m.Router)
		})

		gitHTTP := []macaron.Handler{context.ServeGoGet(), repo.HTTPContexter(repo.NewStore()), repo.HTTP}
		m.Route("/info/refs", "GET,OPTIONS", gitHTTP...)
		m.Route("/HEAD", "GET,OPTIONS", gitHTTP...)
		m.Route("/git-upload-pack", "POST,OPTIONS", gitHTTP...)
		m.Route("/git-receive-pack", "POST,OPTIONS", gitHTTP...)
		m.Route("/objects/info/alternates", "GET,OPTIONS", gitHTTP...)
		m.Route("/objects/info/http-alternates", "GET,OPTIONS", gitHTTP...)
		m.Route("/objects/info/packs", "GET,OPTIONS", gitHTTP...)
		m.Route("/objects/info/*", "GET,OPTIONS", gitHTTP...)
		m.Route("/objects/:prefix([0-9a-f]{2})/:suffix([0-9a-f]{38})", "GET,OPTIONS", gitHTTP...)
		m.Route("/objects/pack/pack-:sha([0-9a-f]{40}).pack", "GET,OPTIONS", gitHTTP...)
		m.Route("/objects/pack/pack-:sha([0-9a-f]{40}).idx", "GET,OPTIONS", gitHTTP...)
	})

	// ***************************
	// ----- Internal routes -----
	// ***************************

	m.Group("/-", func() {
		m.Get("/metrics", app.MetricsFilter(), promhttp.Handler()) // "/-/metrics"

		m.Group("/api", func() {
			m.Post("/sanitize_ipynb", app.SanitizeIpynb()) // "/-/api/sanitize_ipynb"
		})
	})

	// Flag for port number in case first time run conflict.
	if portOverride > 0 {
		port := strconv.Itoa(portOverride)
		conf.Server.URL.Host = strings.Replace(conf.Server.URL.Host, ":"+conf.Server.URL.Port(), ":"+port, 1)
		conf.Server.ExternalURL = conf.Server.URL.String()
		conf.Server.HTTPPort = portOverride
	}

	var listenAddr string
	if conf.Server.Protocol == "unix" {
		listenAddr = conf.Server.HTTPAddr
	} else {
		listenAddr = fmt.Sprintf("%s:%d", conf.Server.HTTPAddr, conf.Server.HTTPPort)
	}
	log.Info("Available on %s", conf.Server.ExternalURL)

	switch conf.Server.Protocol {
	case "http":
		err = http.ListenAndServe(listenAddr, m)

	case "https":
		tlsMinVersion := tls.VersionTLS12
		switch conf.Server.TLSMinVersion {
		case "TLS13":
			tlsMinVersion = tls.VersionTLS13
		case "TLS12":
			tlsMinVersion = tls.VersionTLS12
		case "TLS11":
			tlsMinVersion = tls.VersionTLS11
		case "TLS10":
			tlsMinVersion = tls.VersionTLS10
		}
		server := &http.Server{
			Addr: listenAddr,
			TLSConfig: &tls.Config{
				MinVersion:               uint16(tlsMinVersion),
				CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521},
				PreferServerCipherSuites: true,
				CipherSuites: []uint16{
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				},
			}, Handler: m,
		}
		err = server.ListenAndServeTLS(conf.Server.CertFile, conf.Server.KeyFile)

	case "fcgi":
		err = fcgi.Serve(nil, m)

	case "unix":
		if osx.Exist(listenAddr) {
			err = os.Remove(listenAddr)
			if err != nil {
				return errors.Wrap(err, "remove existing Unix domain socket")
			}
		}

		var listener *net.UnixListener
		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: listenAddr, Net: "unix"})
		if err != nil {
			return errors.Wrap(err, "listen on Unix network")
		}

		// FIXME: add proper implementation of signal capture on all protocols
		// execute this on SIGTERM or SIGINT: listener.Close()
		if err = os.Chmod(listenAddr, conf.Server.UnixSocketMode); err != nil {
			return errors.Wrap(err, "change permission of Unix domain socket")
		}
		err = http.Serve(listener, m)

	default:
		return errors.Newf("unexpected server protocol: %s", conf.Server.Protocol)
	}

	if err != nil {
		return errors.Wrap(err, "start server")
	}

	return nil
}

func newRoutingHandler() (http.Handler, error) {
	f := flamego.New()
	f.Use(recovery())
	f.Use(flamegoInjector)
	f.Use(captcha.Captchaer(captcha.Options{URLPrefix: "/captcha/"}))

	cacherOpts, err := parseCacheOptions(conf.Cache)
	if err != nil {
		return nil, errors.Wrap(err, "parse cache options")
	}
	f.Use(cache.Cacher(cacherOpts))

	f.ReturnHandler(func(c flamego.Context, statusCode int, resp any, err error) {
		w := c.ResponseWriter()
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			msg := err.Error()
			if statusCode >= http.StatusInternalServerError && conf.IsProdMode() {
				msg = "Internal server error"
			}
			resp = map[string]any{"error": msg}
		}
		if resp == nil {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(resp)
	})

	f.Get("/redirect", getRedirect)
	f.Get("/robots.txt", getRobotsTxt)

	// The captcha middleware writes the response. This route exists so the request reaches it.
	f.Get("/captcha/image.jpeg", func() {})

	f.Group("/{owner}/{repo}", func() {
		f.Get("/commit/{sha: /[0-9a-f]{7,40}/}.{format: /(diff|patch)/}", getRepoCommitRaw)
		f.Get("/raw/{ref}/{filepath: **}", getRepoRawFile)
	}, withRepoContext)

	mountWebAPIRoutes(f)
	err = mountWebAppRoutes(f)
	if err != nil {
		return nil, errors.Wrap(err, "mount web app routes")
	}
	return f, nil
}

func getRedirect(c flamego.Context) {
	to := c.Request().URL.Query().Get("to")
	if !urlx.IsSameSite(to) {
		to = conf.Server.Subpath + "/"
	}
	c.Redirect(to, http.StatusSeeOther)
}

func getRobotsTxt(w http.ResponseWriter, r *http.Request) {
	if !conf.HasRobotsTxt {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, filepath.Join(conf.CustomDir(), "robots.txt"))
}

// newMacaron initializes Macaron instance.
func newMacaron() (*macaron.Macaron, error) {
	m := macaron.New()
	if !conf.Server.DisableRouterLog {
		m.Use(macaron.Logger())
	}
	m.Use(macaron.Recovery())
	if conf.Server.EnableGzip {
		m.Use(gzip.Gziper())
	}
	if conf.Server.Protocol == "fcgi" {
		m.SetURLPrefix(conf.Server.Subpath)
	}

	// Register custom middleware first to make it possible to override files under "public".
	m.Use(macaron.Static(
		filepath.Join(conf.CustomDir(), "public"),
		macaron.StaticOptions{
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))
	var publicFs http.FileSystem
	if !conf.Server.LoadAssetsFromDisk {
		publicFs = http.FS(public.Files)
	}
	m.Use(macaron.Static(
		filepath.Join(conf.WorkDir(), "public"),
		macaron.StaticOptions{
			ETag:        true,
			SkipLogging: conf.Server.DisableRouterLog,
			FileSystem:  publicFs,
		},
	))

	m.Use(macaron.Static(
		conf.Picture.AvatarUploadPath,
		macaron.StaticOptions{
			ETag:        true,
			Prefix:      conf.UsersAvatarPathPrefix,
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))
	m.Use(macaron.Static(
		conf.Picture.RepositoryAvatarUploadPath,
		macaron.StaticOptions{
			ETag:        true,
			Prefix:      database.RepoAvatarURLPrefix,
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))

	customDir := filepath.Join(conf.CustomDir(), "templates")
	renderOpt := macaron.RenderOptions{
		Directory:         filepath.Join(conf.WorkDir(), "templates"),
		AppendDirectories: []string{customDir},
		Funcs:             template.FuncMap(),
		IndentJSON:        macaron.Env != macaron.PROD,
	}
	if !conf.Server.LoadAssetsFromDisk {
		renderOpt.TemplateFileSystem = templates.NewTemplateFileSystem("", customDir)
	}
	m.Use(macaron.Renderer(renderOpt))

	localeNames, err := embedconf.FileNames("locale")
	if err != nil {
		return nil, errors.Wrap(err, "list locale files")
	}
	localeFiles := make(map[string][]byte)
	for _, name := range localeNames {
		localeFiles[name], err = embedconf.Files.ReadFile("locale/" + name)
		if err != nil {
			return nil, errors.Wrapf(err, "read locale file %q", name)
		}
	}
	m.Use(i18n.I18n(i18n.Options{
		SubURL:          conf.Server.Subpath,
		Files:           localeFiles,
		CustomDirectory: filepath.Join(conf.CustomDir(), "conf", "locale"),
		Langs:           conf.I18n.Langs,
		Names:           conf.I18n.Names,
		DefaultLang:     "en-US",
		Redirect:        true,
	}))
	m.Use(macaroncache.Cacher(macaroncache.Options{
		Adapter:       conf.Cache.Adapter,
		AdapterConfig: conf.Cache.Host,
		Interval:      conf.Cache.Interval,
	}))
	m.Route("/healthcheck", http.MethodHead+","+http.MethodGet, healthCheck)
	return m, nil
}

// renderIndex returns the index.html shell with per-request substitutions
// applied for the given WebContext.
func renderIndex(index []byte, wc context.WebContext) ([]byte, error) {
	// json.Marshal escapes <, >, and &, so the payload cannot break out of the surrounding <script>.
	payload, err := json.Marshal(struct {
		Lang   string `json:"lang"`
		SubURL string `json:"subURL"`
	}{
		Lang:   wc.Lang,
		SubURL: wc.SubURL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal web context")
	}
	script := `<script>window.__webContext=` + string(payload) +
		`;document.documentElement.lang=window.__webContext.lang;</script>`

	pairs := []string{
		"{{.WebContext}}", script,
	}
	if wc.SubURL != "" {
		// Prefix entrypoint paths with the subpath for non-root mounts. Other
		// bundled assets stay relative to the entrypoint that references them.
		pairs = append(pairs,
			`src="./assets/`, `src="`+wc.SubURL+`/assets/`,
			`href="./assets/`, `href="`+wc.SubURL+`/assets/`,
			`src="/src/`, `src="`+wc.SubURL+`/src/`,
			`href="/img/`, `href="`+wc.SubURL+`/img/`,
		)
	}
	return []byte(strings.NewReplacer(pairs...).Replace(string(index))), nil
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	if err := database.Ping(); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, "* Database connection: %s\n", err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte("* Database connection: OK\n"))
}

func initServices(customConf string) error {
	err := conf.Init(customConf)
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}

	conf.InitLogging(false)
	log.Info("%s %s", conf.App.BrandName, conf.App.Version)
	log.Trace("Work directory: %s", conf.WorkDir())
	log.Trace("Custom path: %s", conf.CustomDir())
	log.Trace("Custom config: %s", conf.CustomConf)
	log.Trace("Log path: %s", conf.Log.RootPath)
	log.Trace("Build time: %s", conf.BuildTime)
	log.Trace("Build commit: %s", conf.BuildCommit)

	if conf.IsProdMode() {
		macaron.Env = macaron.PROD
		flamego.SetEnv(flamego.EnvTypeProd)
		macaron.ColorLog = false
		git.SetOutput(nil)
	} else {
		git.SetOutput(os.Stdout)
	}
	log.Info("Run mode: %s", strings.Title(macaron.Env))

	if conf.Email.Enabled {
		log.Trace("Email service is enabled")
	}

	email.NewContext()

	highlight.NewContext()
	markup.NewSanitizer()
	if err := database.NewEngine(); err != nil {
		return errors.Wrap(err, "initialize ORM engine")
	}

	database.LoadRepoConfig()
	database.NewRepoContext()

	cron.NewContext()
	database.InitSyncMirrors()
	database.InitDeliverHooks()
	database.InitTestPullRequests()

	if conf.HasMinWinSvc {
		log.Info("Builtin Windows Service is supported")
	}
	if conf.Server.LoadAssetsFromDisk {
		log.Trace("Assets are loaded from disk")
	}

	if conf.SSH.StartBuiltinServer {
		ssh.Listen(conf.SSH, conf.Server.AppDataPath)
		log.Info("SSH server started on %s:%v", conf.SSH.ListenHost, conf.SSH.ListenPort)
		log.Trace("SSH server cipher list: %v", conf.SSH.ServerCiphers)
		log.Trace("SSH server MAC list: %v", conf.SSH.ServerMACs)
		log.Trace("SSH server algorithms: %v", conf.SSH.ServerAlgorithms)
	}

	if conf.SSH.RewriteAuthorizedKeysAtStart {
		if err := database.RewriteAuthorizedKeys(); err != nil {
			log.Warn("Failed to rewrite authorized_keys file: %v", err)
		}
	}

	return nil
}
