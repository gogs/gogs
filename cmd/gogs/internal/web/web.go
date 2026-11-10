package web

import (
	gocontext "context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/captcha"
	"github.com/flamego/flamego"
	"github.com/go-macaron/binding"
	macaroncache "github.com/go-macaron/cache"
	"github.com/go-macaron/gzip"
	"github.com/go-macaron/i18n"
	"github.com/go-macaron/session"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/cmd/gogs/internal/tracing"
	embedconf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/app"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/route"
	apiv1 "gogs.io/gogs/internal/route/api/v1"
	"gogs.io/gogs/internal/route/lfs"
	"gogs.io/gogs/internal/route/repo"
	"gogs.io/gogs/internal/route/user"
	"gogs.io/gogs/internal/ssh"
	"gogs.io/gogs/internal/template"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/urlx"
	"gogs.io/gogs/public"
	"gogs.io/gogs/templates"
)

// Run starts the web server with the given configuration path and port
// override, and stops it when the context is done.
func Run(ctx gocontext.Context, configPath string, portOverride int) error {
	err := initServices(configPath)
	if err != nil {
		return errors.Wrap(err, "initialize application")
	}
	shutdownTracing, err := tracing.Init(ctx)
	if err != nil {
		return errors.Wrap(err, "initialize tracing")
	}
	if shutdownTracing != nil {
		// Stopping Gogs must not wait long on a collector that is down.
		defer shutdownTracing(5 * time.Second)
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
			m.Get("/^:type(repos|users|organizations)$", func(c *context.Context) { c.ServeWeb() })
		}, ignSignIn)
		m.Get("/^:type(issues|pulls)$", reqSignIn, func(c *context.Context) { c.ServeWeb() })

		// ***** START: User *****
		m.Group("/user/settings", func() {
			m.Get("", func(c *context.Context) { c.ServeWeb() })
			m.Get("/^:page(avatar|password|email|ssh|security|repositories|organizations|applications|delete)$", func(c *context.Context) { c.ServeWeb() })
			// Like the server-rendered pages, these pages are only found in the
			// two-factor authentication state they apply to.
			m.Get("/security/two_factor_enable", func(c *context.Context) {
				if database.Handle.TwoFactors().IsEnabled(c.Req.Context(), c.User.ID) {
					c.NotFound()
					return
				}
				c.ServeWeb()
			})
			m.Get("/security/two_factor_recovery_codes", func(c *context.Context) {
				if !database.Handle.TwoFactors().IsEnabled(c.Req.Context(), c.User.ID) {
					c.NotFound()
					return
				}
				c.ServeWeb()
			})
		}, reqSignIn, func(c *context.Context) {
			c.Data["PageIsUserSettings"] = true
		})

		m.Group("/user", func() {
			m.Any("/activate_email", user.ActivateEmail)
			m.Get("/email2user", user.Email2User)
			// The pages for signing in stay reachable when the site requires signing in to
			// view content. Without this route they would match "/:username/:reponame" and
			// its sign-in toggle.
			m.Get("/^:page(sign-in|sign-up|mfa|reset-password|activate)$", func(c *context.Context) { c.ServeWeb() })
		})
		// ***** END: User *****

		reqAdmin := context.Toggle(&context.ToggleOptions{SignInRequired: true, AdminRequired: true})

		// ***** START: Admin *****
		m.Group("/admin", func() {
			m.Get("", func(c *context.Context) { c.ServeWeb() })
			m.Get("/^:page(operations|users|orgs|repos|auths|notices|config|monitor)$", func(c *context.Context) { c.ServeWeb() })
			m.Get("/users/new", func(c *context.Context) { c.ServeWeb() })
			m.Get("/users/:userid", func(c *context.Context) {
				user, err := database.Handle.Users().GetByID(c.Req.Context(), c.ParamsInt64(":userid"))
				if err != nil {
					c.NotFoundOrError(err, "get user by ID")
					return
				}
				// Organizations have their own settings.
				if user.IsOrganization() {
					c.NotFound()
					return
				}
				c.ServeWeb()
			})

			m.Get("/auths/new", func(c *context.Context) { c.ServeWeb() })
			m.Get("/auths/:authid", func(c *context.Context) {
				if _, err := database.Handle.LoginSources().GetByID(c.Req.Context(), c.ParamsInt64(":authid")); err != nil {
					c.NotFoundOrError(err, "get login source by ID")
					return
				}
				c.ServeWeb()
			})
		}, reqAdmin)
		// ***** END: Admin *****

		m.Group("", func() {
			m.Group("/:username", func() {
				m.Get("", user.Profile)
				m.Get("/^:type(followers|following)$", func(c *context.Context) { c.ServeWeb() })
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

		// webhookPages serves the webhook settings pages of a repository or an
		// organization. Like the server-rendered pages, only the kinds of webhooks
		// the site allows can be added, and webhooks of others are not found.
		webhookPages := func(lookup func(c *context.Context, id int64) (*database.Webhook, error)) {
			m.Get("", func(c *context.Context) { c.ServeWeb() })
			m.Get("/:type/new", func(c *context.Context) {
				if !slices.Contains(conf.Webhook.Types, c.Params(":type")) {
					c.NotFound()
					return
				}
				c.ServeWeb()
			})
			m.Get("/:id", func(c *context.Context) {
				id := c.ParamsInt64(":id")
				if id <= 0 {
					c.NotFound()
					return
				}
				if _, err := lookup(c, id); err != nil {
					c.NotFoundOrError(err, "get webhook")
					return
				}
				c.ServeWeb()
			})
		}

		// ***** START: Organization *****
		m.Group("/org", func() {
			m.Group("", func() {
				m.Get("/create", func(c *context.Context) { c.ServeWeb() })
			}, func(c *context.Context) {
				if !c.User.CanCreateOrganization() {
					c.NotFound()
				}
			})

			m.Group("/:org", func() {
				m.Get("/dashboard", func(c *context.Context) { c.ServeWeb() })
				m.Get("/^:type(issues|pulls)$", func(c *context.Context) { c.ServeWeb() })
				m.Get("/members", func(c *context.Context) { c.ServeWeb() })
				m.Get("/teams", func(c *context.Context) { c.ServeWeb() })
			}, context.OrgAssignment(true))

			m.Group("/:org", func() {
				m.Get("/teams/:team", func(c *context.Context) { c.ServeWeb() })
				m.Get("/teams/:team/repositories", func(c *context.Context) { c.ServeWeb() })
			}, context.OrgAssignment(true, false, true))

			m.Group("/:org", func() {
				m.Get("/teams/new", func(c *context.Context) { c.ServeWeb() })
				m.Get("/teams/:team/edit", func(c *context.Context) { c.ServeWeb() })

				m.Group("/settings", func() {
					m.Get("", func(c *context.Context) { c.ServeWeb() })
					m.Group("/hooks", func() {
						webhookPages(func(c *context.Context, id int64) (*database.Webhook, error) {
							return database.GetWebhookByOrgID(c.Org.Organization.ID, id)
						})
					})
					m.Get("/delete", func(c *context.Context) { c.ServeWeb() })
				})

				m.Get("/invitations/new", func(c *context.Context) { c.ServeWeb() })
			}, context.OrgAssignment(true, true))
		}, reqSignIn)
		// ***** END: Organization *****

		// ***** START: Repository *****
		m.Group("/repo", func() {
			m.Get("/^:page(create|migrate)$", func(c *context.Context) {
				_, err := repoOwner(c.Req.Context(), c.User, c.QueryInt64("org"))
				if errors.Is(err, errRepoOwnerNotAllowed) {
					c.NotFound()
					return
				} else if err != nil {
					c.Error(err, "get repository owner")
					return
				}
				c.ServeWeb()
			})
			m.Get("/fork/:repoid", func(c *context.Context) {
				_, err := forkBaseRepo(c.User, c.ParamsInt64(":repoid"))
				if errors.Is(err, errForkBaseNotFound) {
					c.NotFound()
					return
				} else if err != nil {
					c.Error(err, "get repository to fork")
					return
				}
				c.ServeWeb()
			})
		}, reqSignIn)

		m.Group("/:username/:reponame", func() {
			m.Group("/settings", func() {
				m.Get("", func(c *context.Context) { c.ServeWeb() })
				m.Get("/collaboration", func(c *context.Context) { c.ServeWeb() })
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
					webhookPages(func(c *context.Context, id int64) (*database.Webhook, error) {
						return database.GetWebhookOfRepoByID(c.Repo.Repository.ID, id)
					})

					m.Group("/git", func() {
						m.Get("", repo.SettingsGitHooks)
						m.Combo("/:name").Get(repo.SettingsGitHooksEdit).
							Post(repo.SettingsGitHooksEditPost)
					}, context.GitHookService())
				})

				m.Get("/keys", func(c *context.Context) { c.ServeWeb() })
			}, func(c *context.Context) {
				c.Data["PageIsSettings"] = true
			})
		}, reqSignIn, context.RepoAssignment(), reqRepoAdmin, context.RepoRef())

		m.Post("/:username/:reponame/action/:action", reqSignIn, context.RepoAssignment(), repo.Action)
		m.Group("/:username/:reponame", func() {
			m.Get("/issues", repo.RetrieveLabels, repo.Issues)
			m.Get("/issues/:index", repo.ViewIssue)
			m.Get("/^:page(labels|milestones)$", func(c *context.Context) { c.ServeWeb() })
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
			m.Group("/milestones", func() {
				m.Get("/new", func(c *context.Context) { c.ServeWeb() })
				m.Get("/:id/edit", func(c *context.Context) {
					// An ID of 0 would match any milestone of the repository.
					id := c.ParamsInt64(":id")
					if id <= 0 {
						c.NotFound()
						return
					}
					if _, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, id); err != nil {
						c.NotFoundOrError(err, "get milestone")
						return
					}
					c.ServeWeb()
				})
			}, reqRepoWriter)

			m.Group("/releases", func() {
				m.Get("/new", repo.NewRelease)
				m.Post("/new", bindIgnErr(form.NewRelease{}), repo.NewReleasePost)
				m.Post("/delete", repo.DeleteRelease)
				m.Get("/edit/*", repo.EditRelease)
				m.Post("/edit/*", bindIgnErr(form.EditRelease{}), repo.EditReleasePost)
			}, repo.MustBeNotEmpty, reqRepoWriter, func(c *context.Context) {
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
			}, repo.MustBeNotEmpty, reqRepoWriter, context.RepoRef(), func(c *context.Context) {
				if !c.Repo.CanEnableEditor() {
					c.NotFound()
					return
				}

				c.Data["PageIsViewFiles"] = true
			})
		}, reqSignIn, context.RepoAssignment())

		m.Group("/:username/:reponame", func() {
			m.Group("", func() {
				m.Get("/releases", repo.MustBeNotEmpty, repo.Releases)
				m.Get("/pulls", repo.RetrieveLabels, repo.Pulls)
				m.Get("/pulls/:index", repo.ViewPull)
			}, context.RepoRef())

			m.Group("/branches", func() {
				m.Get("", repo.Branches)
				m.Get("/all", repo.AllBranches)
				m.Post("/delete/*", reqSignIn, reqRepoWriter, repo.DeleteBranchPost)
			}, repo.MustBeNotEmpty, func(c *context.Context) {
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

			m.Get("/archive/*", repo.MustBeNotEmpty, repo.Download)

			m.Group("/pulls/:index", func() {
				m.Get("/commits", context.RepoRef(), repo.ViewPullCommits)
				m.Get("/files", context.RepoRef(), repo.ViewPullFiles)
				m.Post("/merge", reqRepoWriter, repo.MergePullRequest)
			}, repo.MustAllowPulls)

			m.Group("", func() {
				m.Get("/src/*", repo.Home)
				m.Get("/commits/*", repo.RefCommits)
				m.Get("/forks", repo.Forks)
			}, repo.MustBeNotEmpty, context.RepoRef())
			// Bridged to Flamego to skip the legacy `RepoRef` middleware, which double-resolves the ref.
			m.Get("/raw/*", flamegoBridger(webHandler))
			m.Get("/commit/:sha([a-f0-9]{7,64})\\.:ext(patch|diff)", flamegoBridger(webHandler))
			// Constrain SHA shape so non-matching `/commit/...` paths 404 instead of loading the SPA with a bad param.
			m.Get("/commit/:sha([a-f0-9]{7,64})$", repo.MustBeNotEmpty, func(c *context.Context) { c.ServeWeb() })

			m.Get("/compare/:before([a-z0-9]{64}|[a-z0-9]{40})\\.\\.\\.:after([a-z0-9]{64}|[a-z0-9]{40})", repo.MustBeNotEmpty, context.RepoRef(), repo.CompareDiff)
		}, ignSignIn, context.RepoAssignment())
		m.Group("/:username/:reponame", func() {
			m.Get("", repo.Home)
			m.Get("/^:type(stars|watchers)$", func(c *context.Context) { c.ServeWeb() })
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
		m.Route("/git-upload-pack", "POST,OPTIONS", gitHTTP...)
		m.Route("/git-receive-pack", "POST,OPTIONS", gitHTTP...)
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

	var handler http.Handler = m
	if shutdownTracing != nil {
		handler = newTracingHandler(m)
	}

	server := &http.Server{Addr: listenAddr, Handler: handler}
	serve := server.ListenAndServe
	stop := server.Close
	switch conf.Server.Protocol {
	case "http":

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
		server.TLSConfig = &tls.Config{
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
		}
		serve = func() error {
			return server.ListenAndServeTLS(conf.Server.CertFile, conf.Server.KeyFile)
		}

	case "fcgi":
		// The web server is started by a FastCGI server, which passes the listener
		// as standard input.
		listener, err := net.FileListener(os.Stdin)
		if err != nil {
			return errors.Wrap(err, "listen on standard input")
		}
		serve = func() error { return fcgi.Serve(listener, handler) }
		stop = listener.Close

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

		if err = os.Chmod(listenAddr, conf.Server.UnixSocketMode); err != nil {
			return errors.Wrap(err, "change permission of Unix domain socket")
		}
		serve = func() error { return server.Serve(listener) }

	default:
		return errors.Newf("unexpected server protocol: %s", conf.Server.Protocol)
	}

	served := make(chan error, 1)
	go func() {
		served <- serve()
	}()
	select {
	case err = <-served:
		return errors.Wrap(err, "start server")
	case <-ctx.Done():
		if err = stop(); err != nil {
			return errors.Wrap(err, "stop server")
		}
		return nil
	}
}

// newTracingHandler returns a handler that records a span for every request, so
// that the spans of what the request runs, e.g., Git operations, are grouped
// under it.
func newTracingHandler(h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, "", otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if r.Pattern != "" {
			return r.Pattern
		}
		return r.Method + " " + r.URL.Path
	}))
}

func newRoutingHandler() (http.Handler, error) {
	f := flamego.New()
	f.Use(flamego.Recovery(flamego.RecoveryOptions{PlainText: true}))
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
		f.Get("/commit/{sha: /[0-9a-f]{7,64}/}.{format: /(diff|patch)/}", getRepoCommitRaw)
		f.Get("/raw/{ref}/{filepath: **}", getRepoRawFile)
	}, requireViewAccess, withRepoContext)

	mountWebLocaleRoutes(f)
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
	customLocaleDir := filepath.Join(conf.CustomDir(), "conf", "locale")
	m.Use(i18n.I18n(i18n.Options{
		SubURL:          conf.Server.Subpath,
		Files:           localeFiles,
		CustomDirectory: customLocaleDir,
		Langs:           conf.I18n.Langs,
		Names:           conf.I18n.Names,
		DefaultLang:     "en-US",
		Redirect:        true,
	}))
	webLocales, err = loadWebLocales(customLocaleDir, conf.I18n.Langs)
	if err != nil {
		return nil, errors.Wrap(err, "load web locales")
	}
	m.Use(macaroncache.Cacher(macaroncache.Options{
		Adapter:       conf.Cache.Adapter,
		AdapterConfig: conf.Cache.Host,
		Interval:      conf.Cache.Interval,
	}))
	m.Route("/healthcheck", http.MethodHead+","+http.MethodGet, healthCheck)
	return m, nil
}

// injectContent holds the custom content spliced into the React shell.
type injectContent struct {
	head   string
	footer string
}

// readInjectContent reads the head and footer injection templates from the
// custom directory, falling back to the empty embedded defaults.
func readInjectContent() (injectContent, error) {
	customDir := filepath.Join(conf.CustomDir(), "templates")
	head, err := templates.ReadInjectFile(customDir, "head.tmpl")
	if err != nil {
		return injectContent{}, errors.Wrap(err, "read head injection")
	}
	footer, err := templates.ReadInjectFile(customDir, "footer.tmpl")
	if err != nil {
		return injectContent{}, errors.Wrap(err, "read footer injection")
	}
	return injectContent{head: string(head), footer: string(footer)}, nil
}

// renderIndex returns the index.html shell with per-request substitutions
// applied for the given WebContext. The inject argument supplies the custom
// head and footer content to splice into the shell.
func renderIndex(index []byte, wc context.WebContext, inject injectContent) ([]byte, error) {
	// json.Marshal escapes <, >, and &, so the payload cannot break out of the surrounding <script>.
	payload, err := json.Marshal(struct {
		Lang      string `json:"lang"`
		SubURL    string `json:"subURL"`
		LocaleURL string `json:"localeURL"`
		Page      string `json:"page,omitempty"`
		Error     string `json:"error,omitempty"`
		Version   string `json:"version,omitempty"`
	}{
		Lang:      wc.Lang,
		SubURL:    wc.SubURL,
		LocaleURL: wc.SubURL + webLocaleURL(wc.Lang),
		Page:      wc.Page,
		Error:     wc.Error,
		Version:   wc.Version,
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal web context")
	}
	// Start loading the translations alongside the bundle that needs them.
	script := `<link rel="preload" as="fetch" crossorigin href="` + html.EscapeString(wc.SubURL+webLocaleURL(wc.Lang)) + `">` +
		`<script>window.__webContext=` + string(payload) +
		`;document.documentElement.lang=window.__webContext.lang;</script>`

	pairs := []string{
		// Append the head injection after the web context script so custom
		// tags land inside <head>. The footer injection replaces the closing
		// body tag to land just before </body>.
		"{{.WebContext}}", script + inject.head,
		"</body>", inject.footer + "</body>",
		// Make entrypoint paths absolute, so pages nested below the root (e.g.,
		// "/user/sign-in") load them from the assets directory rather than one
		// relative to the page. Other bundled assets stay relative to the
		// entrypoint that references them.
		`src="./assets/`, `src="` + wc.SubURL + `/assets/`,
		`href="./assets/`, `href="` + wc.SubURL + `/assets/`,
	}
	if wc.SubURL != "" {
		// Prefix Vite's absolute root paths with the subpath for non-root mounts.
		pairs = append(pairs,
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
