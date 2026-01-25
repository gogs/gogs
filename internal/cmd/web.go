package cmd

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"strings"

	"github.com/flamego/binding"
	"github.com/flamego/cache"
	"github.com/flamego/captcha"
	"github.com/flamego/csrf"
	"github.com/flamego/flamego"
	"github.com/flamego/gzip"
	"github.com/flamego/i18n"
	"github.com/flamego/session"
	"github.com/flamego/template"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/unknwon/com"
	"github.com/urfave/cli"
	log "unknwon.dev/clog/v2"

	embedConf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/app"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/route"
	"gogs.io/gogs/internal/route/admin"
	apiv1 "gogs.io/gogs/internal/route/api/v1"
	"gogs.io/gogs/internal/route/dev"
	"gogs.io/gogs/internal/route/lfs"
	"gogs.io/gogs/internal/route/org"
	"gogs.io/gogs/internal/route/repo"
	"gogs.io/gogs/internal/route/user"
	gogstemplate "gogs.io/gogs/internal/template"
	"gogs.io/gogs/public"
	"gogs.io/gogs/templates"
)

var Web = cli.Command{
	Name:  "web",
	Usage: "Start web server",
	Description: `Gogs web server is the only thing you need to run,
and it takes care of all the other things for you`,
	Action: runWeb,
	Flags: []cli.Flag{
		stringFlag("port, p", "3000", "Temporary port number to prevent conflict"),
		stringFlag("config, c", "", "Custom configuration file path"),
	},
}

// newFlamego initializes Flamego instance.
func newFlamego() *flamego.Flame {
	f := flamego.New()
	if !conf.Server.DisableRouterLog {
		f.Use(flamego.Logger())
	}
	f.Use(flamego.Recovery())
	if conf.Server.EnableGzip {
		f.Use(gzip.Gzip())
	}
	if conf.Server.Protocol == "fcgi" {
		f.SetURLPrefix(conf.Server.Subpath)
	}

	// Register custom middleware first to make it possible to override files under "public".
	f.Use(flamego.Static(
		flamego.StaticOptions{
			Directory:   filepath.Join(conf.CustomDir(), "public"),
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))
	var publicFs http.FileSystem
	if !conf.Server.LoadAssetsFromDisk {
		publicFs = http.FS(public.Files)
	}
	f.Use(flamego.Static(
		flamego.StaticOptions{
			Directory:   filepath.Join(conf.WorkDir(), "public"),
			ETag:        true,
			SkipLogging: conf.Server.DisableRouterLog,
			FileSystem:  publicFs,
		},
	))

	f.Use(flamego.Static(
		flamego.StaticOptions{
			Directory:   conf.Picture.AvatarUploadPath,
			ETag:        true,
			Prefix:      conf.UsersAvatarPathPrefix,
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))
	f.Use(flamego.Static(
		flamego.StaticOptions{
			Directory:   conf.Picture.RepositoryAvatarUploadPath,
			ETag:        true,
			Prefix:      database.RepoAvatarURLPrefix,
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))

	customDir := filepath.Join(conf.CustomDir(), "templates")
	renderOpt := template.Options{
		Directory:         filepath.Join(conf.WorkDir(), "templates"),
		AppendDirectories: []string{customDir},
		Funcs:             gogstemplate.FuncMap(),
		FileSystem:        nil,
	}
	if !conf.Server.LoadAssetsFromDisk {
		renderOpt.FileSystem = templates.NewTemplateFileSystem("", customDir)
	}
	f.Use(template.Templater(renderOpt))

	localeNames, err := embedConf.FileNames("locale")
	if err != nil {
		log.Fatal("Failed to list locale files: %v", err)
	}
	localeFiles := make(map[string][]byte)
	for _, name := range localeNames {
		localeFiles[name], err = embedConf.Files.ReadFile("locale/" + name)
		if err != nil {
			log.Fatal("Failed to read locale file %q: %v", name, err)
		}
	}
	f.Use(i18n.I18n(i18n.Options{
		Directory:       filepath.Join(conf.CustomDir(), "conf", "locale"),
		Files:           localeFiles,
		Languages:       conf.I18n.Langs,
		Names:           conf.I18n.Names,
		DefaultLanguage: "en-US",
		Redirect:        true,
	}))
	f.Use(cache.Cacher(cache.Options{
		Adapter:  conf.Cache.Adapter,
		Config:   conf.Cache.Host,
		Interval: conf.Cache.Interval,
	}))
	f.Use(captcha.Captchaer(captcha.Options{
		URLPrefix: conf.Server.Subpath,
	}))

	// Custom health check endpoint (replaces toolbox)
	f.Get("/-/healthz", func(w http.ResponseWriter) {
		if err := database.Ping(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "database connection failed: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	return f
}

func runWeb(c *cli.Context) error {
	err := route.GlobalInit(c.String("config"))
	if err != nil {
		log.Fatal("Failed to initialize application: %v", err)
	}

	f := newFlamego()

	// Apply global middleware
	f.Use(session.Sessioner(session.Options{
		Config: session.MemoryConfig{},
		Cookie: session.CookieOptions{
			Name:   conf.Session.CookieName,
			Path:   conf.Server.Subpath,
			MaxAge: int(conf.Session.MaxLifeTime),
			Secure: conf.Session.CookieSecure,
		},
	}))
	f.Use(csrf.Csrfer(csrf.Options{
		Secret: conf.Security.SecretKey,
		Header: "X-CSRF-Token",
	}))
	f.Use(context.Contexter(context.NewStore()))

	reqSignIn := context.Toggle(&context.ToggleOptions{SignInRequired: true})
	ignSignIn := context.Toggle(&context.ToggleOptions{SignInRequired: conf.Auth.RequireSigninView})
	reqSignOut := context.Toggle(&context.ToggleOptions{SignOutRequired: true})

	f.Get("/", ignSignIn, route.Home)
	f.Group("/explore", func() {
		f.Get("", func(c *context.Context) {
			c.Redirect(conf.Server.Subpath + "/explore/repos")
		})
		f.Get("/repos", route.ExploreRepos)
		f.Get("/users", route.ExploreUsers)
		f.Get("/organizations", route.ExploreOrganizations)
	}, ignSignIn)
	f.Combo("/install", route.InstallInit).Get(route.Install).
		Post(binding.Form(form.Install{}), route.InstallPost)
	f.Get("/<type:issues|pulls>", reqSignIn, user.Issues)

	// ***** START: User *****
	f.Group("/user", func() {
		f.Group("/login", func() {
			f.Combo("").Get(user.Login).
				Post(binding.Form(form.SignIn{}), user.LoginPost)
			f.Combo("/two_factor").Get(user.LoginTwoFactor).Post(user.LoginTwoFactorPost)
			f.Combo("/two_factor_recovery_code").Get(user.LoginTwoFactorRecoveryCode).Post(user.LoginTwoFactorRecoveryCodePost)
		})

		f.Get("/sign_up", user.SignUp)
		f.Post("/sign_up", binding.Form(form.Register{}), user.SignUpPost)
		f.Get("/reset_password", user.ResetPasswd)
		f.Post("/reset_password", user.ResetPasswdPost)
	}, reqSignOut)

	f.Group("/user/settings", func() {
		f.Get("", user.Settings)
		f.Post("", binding.Form(form.UpdateProfile{}), user.SettingsPost)
		f.Combo("/avatar").Get(user.SettingsAvatar).
			Post(binding.Form(form.Avatar{}), user.SettingsAvatarPost)
		f.Post("/avatar/delete", user.SettingsDeleteAvatar)
		f.Combo("/email").Get(user.SettingsEmails).
			Post(binding.Form(form.AddEmail{}), user.SettingsEmailPost)
		f.Post("/email/delete", user.DeleteEmail)
		f.Get("/password", user.SettingsPassword)
		f.Post("/password", binding.Form(form.ChangePassword{}), user.SettingsPasswordPost)
		f.Combo("/ssh").Get(user.SettingsSSHKeys).
			Post(binding.Form(form.AddSSHKey{}), user.SettingsSSHKeysPost)
		f.Post("/ssh/delete", user.DeleteSSHKey)
		f.Group("/security", func() {
			f.Get("", user.SettingsSecurity)
			f.Combo("/two_factor_enable").Get(user.SettingsTwoFactorEnable).
				Post(user.SettingsTwoFactorEnablePost)
			f.Combo("/two_factor_recovery_codes").Get(user.SettingsTwoFactorRecoveryCodes).
				Post(user.SettingsTwoFactorRecoveryCodesPost)
			f.Post("/two_factor_disable", user.SettingsTwoFactorDisable)
		})
		f.Group("/repositories", func() {
			f.Get("", user.SettingsRepos)
			f.Post("/leave", user.SettingsLeaveRepo)
		})
		f.Group("/organizations", func() {
			f.Get("", user.SettingsOrganizations)
			f.Post("/leave", user.SettingsLeaveOrganization)
		})

		settingsHandler := user.NewSettingsHandler(user.NewSettingsStore())
		f.Combo("/applications").Get(settingsHandler.Applications()).
			Post(binding.Form(form.NewAccessToken{}), settingsHandler.ApplicationsPost())
		f.Post("/applications/delete", settingsHandler.DeleteApplication())
		f.Route("/delete", "GET,POST", user.SettingsDelete)
	}, reqSignIn, func(c *context.Context) {
		c.Data["PageIsUserSettings"] = true
	})

	f.Group("/user", func() {
		f.Any("/activate", user.Activate)
		f.Any("/activate_email", user.ActivateEmail)
		f.Get("/email2user", user.Email2User)
		f.Get("/forget_password", user.ForgotPasswd)
		f.Post("/forget_password", user.ForgotPasswdPost)
		f.Post("/logout", user.SignOut)
	})
	// ***** END: User *****

	reqAdmin := context.Toggle(&context.ToggleOptions{SignInRequired: true, AdminRequired: true})

	// ***** START: Admin *****
	f.Group("/admin", func() {
		f.Combo("").Get(admin.Dashboard).Post(admin.Operation) // "/admin"
		f.Get("/config", admin.Config)
		f.Post("/config/test_mail", admin.SendTestMail)
		f.Get("/monitor", admin.Monitor)

		f.Group("/users", func() {
			f.Get("", admin.Users)
			f.Combo("/new").Get(admin.NewUser).Post(binding.Form(form.AdminCrateUser{}), admin.NewUserPost)
			f.Combo("/<userid>").Get(admin.EditUser).Post(binding.Form(form.AdminEditUser{}), admin.EditUserPost)
			f.Post("/<userid>/delete", admin.DeleteUser)
		})

		f.Group("/orgs", func() {
			f.Get("", admin.Organizations)
		})

		f.Group("/repos", func() {
			f.Get("", admin.Repos)
			f.Post("/delete", admin.DeleteRepo)
		})

		f.Group("/auths", func() {
			f.Get("", admin.Authentications)
			f.Combo("/new").Get(admin.NewAuthSource).Post(binding.Form(form.Authentication{}), admin.NewAuthSourcePost)
			f.Combo("/<authid>").Get(admin.EditAuthSource).
				Post(binding.Form(form.Authentication{}), admin.EditAuthSourcePost)
			f.Post("/<authid>/delete", admin.DeleteAuthSource)
		})

		f.Group("/notices", func() {
			f.Get("", admin.Notices)
			f.Post("/delete", admin.DeleteNotices)
			f.Get("/empty", admin.EmptyNotices)
		})
	}, reqAdmin)
	// ***** END: Admin *****

	f.Group("", func() {
		f.Group("/<username>", func() {
			f.Get("", user.Profile)
			f.Get("/followers", user.Followers)
			f.Get("/following", user.Following)
			f.Get("/stars", user.Stars)
		}, context.InjectParamsUser())

		f.Get("/attachments/<uuid>", func(c *context.Context) {
			attach, err := database.GetAttachmentByUUID(c.Param("uuid"))
			if err != nil {
				c.NotFoundOrError(err, "get attachment by UUID")
				return
			} else if !com.IsFile(attach.LocalPath()) {
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
			c.Header().Set("Cache-Control", "public,max-age=86400")
			c.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, attach.Name))

			if _, err = io.Copy(c.Resp, fr); err != nil {
				c.Error(err, "copy from file to response")
				return
			}
		})
		f.Post("/issues/attachments", repo.UploadIssueAttachment)
		f.Post("/releases/attachments", repo.UploadReleaseAttachment)
	}, ignSignIn)

	f.Group("/<username>", func() {
		f.Post("/action/<action>", user.Action)
	}, reqSignIn, context.InjectParamsUser())

	if conf.IsProdMode() {
		f.Get("/template/*", dev.TemplatePreview)
	}

	reqRepoAdmin := context.RequireRepoAdmin()
	reqRepoWriter := context.RequireRepoWriter()

	webhookRoutes := func() {
		f.Group("", func() {
			f.Get("", repo.Webhooks)
			f.Post("/delete", repo.DeleteWebhook)
			f.Get("/<type>/new", repo.WebhooksNew)
			f.Post("/gogs/new", binding.Form(form.NewWebhook{}), repo.WebhooksNewPost)
			f.Post("/slack/new", binding.Form(form.NewSlackHook{}), repo.WebhooksSlackNewPost)
			f.Post("/discord/new", binding.Form(form.NewDiscordHook{}), repo.WebhooksDiscordNewPost)
			f.Post("/dingtalk/new", binding.Form(form.NewDingtalkHook{}), repo.WebhooksDingtalkNewPost)
			f.Get("/<id>", repo.WebhooksEdit)
			f.Post("/gogs/<id>", binding.Form(form.NewWebhook{}), repo.WebhooksEditPost)
			f.Post("/slack/<id>", binding.Form(form.NewSlackHook{}), repo.WebhooksSlackEditPost)
			f.Post("/discord/<id>", binding.Form(form.NewDiscordHook{}), repo.WebhooksDiscordEditPost)
			f.Post("/dingtalk/<id>", binding.Form(form.NewDingtalkHook{}), repo.WebhooksDingtalkEditPost)
		}, repo.InjectOrgRepoContext())
	}

	// ***** START: Organization *****
	f.Group("/org", func() {
		f.Group("", func() {
			f.Get("/create", org.Create)
			f.Post("/create", binding.Form(form.CreateOrg{}), org.CreatePost)
		}, func(c *context.Context) {
			if !c.User.CanCreateOrganization() {
				c.NotFound()
			}
		})

		f.Group("/<org>", func() {
			f.Get("/dashboard", user.Dashboard)
			f.Get("/<type:issues|pulls>", user.Issues)
			f.Get("/members", org.Members)
			f.Get("/members/action/<action>", org.MembersAction)

			f.Get("/teams", org.Teams)
		}, context.OrgAssignment(true))

		f.Group("/<org>", func() {
			f.Get("/teams/<team>", org.TeamMembers)
			f.Get("/teams/<team>/repositories", org.TeamRepositories)
			f.Route("/teams/<team>/action/<action>", "GET,POST", org.TeamsAction)
			f.Route("/teams/<team>/action/repo/<action>", "GET,POST", org.TeamsRepoAction)
		}, context.OrgAssignment(true, false, true))

		f.Group("/<org>", func() {
			f.Get("/teams/new", org.NewTeam)
			f.Post("/teams/new", binding.Form(form.CreateTeam{}), org.NewTeamPost)
			f.Get("/teams/<team>/edit", org.EditTeam)
			f.Post("/teams/<team>/edit", binding.Form(form.CreateTeam{}), org.EditTeamPost)
			f.Post("/teams/<team>/delete", org.DeleteTeam)

			f.Group("/settings", func() {
				f.Combo("").Get(org.Settings).
					Post(binding.Form(form.UpdateOrgSetting{}), org.SettingsPost)
				f.Post("/avatar", binding.Form(form.Avatar{}), org.SettingsAvatar)
				f.Post("/avatar/delete", org.SettingsDeleteAvatar)
				f.Group("/hooks", webhookRoutes)
				f.Route("/delete", "GET,POST", org.SettingsDelete)
			})

			f.Route("/invitations/new", "GET,POST", org.Invitation)
		}, context.OrgAssignment(true, true))
	}, reqSignIn)
	// ***** END: Organization *****

	// ***** START: Repository *****
	f.Group("/repo", func() {
		f.Get("/create", repo.Create)
		f.Post("/create", binding.Form(form.CreateRepo{}), repo.CreatePost)
		f.Get("/migrate", repo.Migrate)
		f.Post("/migrate", binding.Form(form.MigrateRepo{}), repo.MigratePost)
		f.Combo("/fork/<repoid>").Get(repo.Fork).
			Post(binding.Form(form.CreateRepo{}), repo.ForkPost)
	}, reqSignIn)

	f.Group("/<username>/<reponame>", func() {
		f.Group("/settings", func() {
			f.Combo("").Get(repo.Settings).
				Post(binding.Form(form.RepoSetting{}), repo.SettingsPost)
			f.Combo("/avatar").Get(repo.SettingsAvatar).
				Post(binding.Form(form.Avatar{}), repo.SettingsAvatarPost)
			f.Post("/avatar/delete", repo.SettingsDeleteAvatar)
			f.Group("/collaboration", func() {
				f.Combo("").Get(repo.SettingsCollaboration).Post(repo.SettingsCollaborationPost)
				f.Post("/access_mode", repo.ChangeCollaborationAccessMode)
				f.Post("/delete", repo.DeleteCollaboration)
			})
			f.Group("/branches", func() {
				f.Get("", repo.SettingsBranches)
				f.Post("/default_branch", repo.UpdateDefaultBranch)
				f.Combo("/*").Get(repo.SettingsProtectedBranch).
					Post(binding.Form(form.ProtectBranch{}), repo.SettingsProtectedBranchPost)
			}, func(c *context.Context) {
				if c.Repo.Repository.IsMirror {
					c.NotFound()
					return
				}
			})

			f.Group("/hooks", func() {
				webhookRoutes()

				f.Group("/<id>", func() {
					f.Post("/test", repo.TestWebhook)
					f.Post("/redelivery", repo.RedeliveryWebhook)
				})

				f.Group("/git", func() {
					f.Get("", repo.SettingsGitHooks)
					f.Combo("/<name>").Get(repo.SettingsGitHooksEdit).
						Post(repo.SettingsGitHooksEditPost)
				}, context.GitHookService())
			})

			f.Group("/keys", func() {
				f.Combo("").Get(repo.SettingsDeployKeys).
					Post(binding.Form(form.AddSSHKey{}), repo.SettingsDeployKeysPost)
				f.Post("/delete", repo.DeleteDeployKey)
			})
		}, func(c *context.Context) {
			c.Data["PageIsSettings"] = true
		})
	}, reqSignIn, context.RepoAssignment(), reqRepoAdmin, context.RepoRef())

	f.Post("/<username>/<reponame>/action/<action>", reqSignIn, context.RepoAssignment(), repo.Action)
	f.Group("/<username>/<reponame>", func() {
		f.Get("/issues", repo.RetrieveLabels, repo.Issues)
		f.Get("/issues/<index>", repo.ViewIssue)
		f.Get("/labels/", repo.RetrieveLabels, repo.Labels)
		f.Get("/milestones", repo.Milestones)
	}, ignSignIn, context.RepoAssignment(true))
	f.Group("/<username>/<reponame>", func() {
		// FIXME: should use different URLs but mostly same logic for comments of issue and pull reuqest.
		// So they can apply their own enable/disable logic on routers.
		f.Group("/issues", func() {
			f.Combo("/new", repo.MustEnableIssues).Get(context.RepoRef(), repo.NewIssue).
				Post(binding.Form(form.NewIssue{}), repo.NewIssuePost)

			f.Group("/<index>", func() {
				f.Post("/title", repo.UpdateIssueTitle)
				f.Post("/content", repo.UpdateIssueContent)
				f.Combo("/comments").Post(binding.Form(form.CreateComment{}), repo.NewComment)
			})
		})
		f.Group("/comments/<id>", func() {
			f.Post("", repo.UpdateCommentContent)
			f.Post("/delete", repo.DeleteComment)
		})
	}, reqSignIn, context.RepoAssignment(true))
	f.Group("/<username>/<reponame>", func() {
		f.Group("/wiki", func() {
			f.Get("/?<page>", repo.Wiki)
			f.Get("/_pages", repo.WikiPages)
		}, repo.MustEnableWiki, context.RepoRef())
	}, ignSignIn, context.RepoAssignment(false, true))

	f.Group("/<username>/<reponame>", func() {
		// FIXME: should use different URLs but mostly same logic for comments of issue and pull reuqest.
		// So they can apply their own enable/disable logic on routers.
		f.Group("/issues", func() {
			f.Group("/<index>", func() {
				f.Post("/label", repo.UpdateIssueLabel)
				f.Post("/milestone", repo.UpdateIssueMilestone)
				f.Post("/assignee", repo.UpdateIssueAssignee)
			}, reqRepoWriter)
		})
		f.Group("/labels", func() {
			f.Post("/new", binding.Form(form.CreateLabel{}), repo.NewLabel)
			f.Post("/edit", binding.Form(form.CreateLabel{}), repo.UpdateLabel)
			f.Post("/delete", repo.DeleteLabel)
			f.Post("/initialize", binding.Form(form.InitializeLabels{}), repo.InitializeLabels)
		}, reqRepoWriter, context.RepoRef())
		f.Group("/milestones", func() {
			f.Combo("/new").Get(repo.NewMilestone).
				Post(binding.Form(form.CreateMilestone{}), repo.NewMilestonePost)
			f.Get("/<id>/edit", repo.EditMilestone)
			f.Post("/<id>/edit", binding.Form(form.CreateMilestone{}), repo.EditMilestonePost)
			f.Get("/<id>/<action>", repo.ChangeMilestonStatus)
			f.Post("/delete", repo.DeleteMilestone)
		}, reqRepoWriter, context.RepoRef())

		f.Group("/releases", func() {
			f.Get("/new", repo.NewRelease)
			f.Post("/new", binding.Form(form.NewRelease{}), repo.NewReleasePost)
			f.Post("/delete", repo.DeleteRelease)
			f.Get("/edit/*", repo.EditRelease)
			f.Post("/edit/*", binding.Form(form.EditRelease{}), repo.EditReleasePost)
		}, repo.MustBeNotBare, reqRepoWriter, func(c *context.Context) {
			c.Data["PageIsViewFiles"] = true
		})

		// FIXME: Should use c.Repo.PullRequest to unify template, currently we have inconsistent URL
		// for PR in same repository. After select branch on the page, the URL contains redundant head user name.
		// e.g. /org1/test-repo/compare/master...org1:develop
		// which should be /org1/test-repo/compare/master...develop
		f.Combo("/compare/*", repo.MustAllowPulls).Get(repo.CompareAndPullRequest).
			Post(binding.Form(form.NewIssue{}), repo.CompareAndPullRequestPost)

		f.Group("", func() {
			f.Combo("/_edit/*").Get(repo.EditFile).
				Post(binding.Form(form.EditRepoFile{}), repo.EditFilePost)
			f.Combo("/_new/*").Get(repo.NewFile).
				Post(binding.Form(form.EditRepoFile{}), repo.NewFilePost)
			f.Post("/_preview/*", binding.Form(form.EditPreviewDiff{}), repo.DiffPreviewPost)
			f.Combo("/_delete/*").Get(repo.DeleteFile).
				Post(binding.Form(form.DeleteRepoFile{}), repo.DeleteFilePost)

			f.Group("", func() {
				f.Combo("/_upload/*").Get(repo.UploadFile).
					Post(binding.Form(form.UploadRepoFile{}), repo.UploadFilePost)
				f.Post("/upload-file", repo.UploadFileToServer)
				f.Post("/upload-remove", binding.Form(form.RemoveUploadFile{}), repo.RemoveUploadFileFromServer)
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

	f.Group("/<username>/<reponame>", func() {
		f.Group("", func() {
			f.Get("/releases", repo.MustBeNotBare, repo.Releases)
			f.Get("/pulls", repo.RetrieveLabels, repo.Pulls)
			f.Get("/pulls/<index>", repo.ViewPull)
		}, context.RepoRef())

		f.Group("/branches", func() {
			f.Get("", repo.Branches)
			f.Get("/all", repo.AllBranches)
			f.Post("/delete/*", reqSignIn, reqRepoWriter, repo.DeleteBranchPost)
		}, repo.MustBeNotBare, func(c *context.Context) {
			c.Data["PageIsViewFiles"] = true
		})

		f.Group("/wiki", func() {
			f.Group("", func() {
				f.Combo("/_new").Get(repo.NewWiki).
					Post(binding.Form(form.NewWiki{}), repo.NewWikiPost)
				f.Combo("/<page>/_edit").Get(repo.EditWiki).
					Post(binding.Form(form.NewWiki{}), repo.EditWikiPost)
				f.Post("/<page>/delete", repo.DeleteWikiPagePost)
			}, reqSignIn, reqRepoWriter)
		}, repo.MustEnableWiki, context.RepoRef())

		f.Get("/archive/*", repo.MustBeNotBare, repo.Download)

		f.Group("/pulls/<index>", func() {
			f.Get("/commits", context.RepoRef(), repo.ViewPullCommits)
			f.Get("/files", context.RepoRef(), repo.ViewPullFiles)
			f.Post("/merge", reqRepoWriter, repo.MergePullRequest)
		}, repo.MustAllowPulls)

		f.Group("", func() {
			f.Get("/src/*", repo.Home)
			f.Get("/raw/*", repo.SingleDownload)
			f.Get("/commits/*", repo.RefCommits)
			f.Get("/commit/<sha:[a-f0-9]{7,40}>", repo.Diff)
			f.Get("/forks", repo.Forks)
		}, repo.MustBeNotBare, context.RepoRef())
		f.Get("/commit/<sha:[a-f0-9]{7,40}>.<ext:patch|diff>", repo.MustBeNotBare, repo.RawDiff)

		f.Get("/compare/<before>([a-z0-9]{40})\\.\\.\\.<after>([a-z0-9]{40})", repo.MustBeNotBare, context.RepoRef(), repo.CompareDiff)
	}, ignSignIn, context.RepoAssignment())
	f.Group("/<username>/<reponame>", func() {
		f.Get("", repo.Home)
		f.Get("/stars", repo.Stars)
		f.Get("/watchers", repo.Watchers)
	}, context.ServeGoGet(), ignSignIn, context.RepoAssignment(), context.RepoRef())
	// ***** END: Repository *****

	// **********************
	// ----- API routes -----
	// **********************

	// TODO: Without session and CSRF
	f.Group("/api", func() {
		apiv1.RegisterRoutes(f)
	}, ignSignIn)

	// ***************************
	// ----- HTTP Git routes -----
	// ***************************

	f.Group("/<username>/<reponame>", func() {
		f.Get("/tasks/trigger", repo.TriggerTask)

		f.Group("/info/lfs", func() {
			lfs.RegisterRoutes(f)
		})

		f.Route("/*", "GET,POST,OPTIONS", context.ServeGoGet(), repo.HTTPContexter(repo.NewStore()), repo.HTTP)
	})

	// ***************************
	// ----- Internal routes -----
	// ***************************

	f.Group("/-", func() {
		f.Get("/metrics", app.MetricsFilter(), promhttp.Handler()) // "/-/metrics"

		f.Group("/api", func() {
			f.Post("/sanitize_ipynb", app.SanitizeIpynb()) // "/-/api/sanitize_ipynb"
		})
	})

	// **********************
	// ----- robots.txt -----
	// **********************

	f.Get("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		if conf.HasRobotsTxt {
			http.ServeFile(w, r, filepath.Join(conf.CustomDir(), "robots.txt"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f.NotFound(route.NotFound)

	// Flag for port number in case first time run conflict.
	if c.IsSet("port") {
		conf.Server.URL.Host = strings.Replace(conf.Server.URL.Host, ":"+conf.Server.URL.Port(), ":"+c.String("port"), 1)
		conf.Server.ExternalURL = conf.Server.URL.String()
		conf.Server.HTTPPort = c.String("port")
	}

	var listenAddr string
	if conf.Server.Protocol == "unix" {
		listenAddr = conf.Server.HTTPAddr
	} else {
		listenAddr = fmt.Sprintf("%s:%s", conf.Server.HTTPAddr, conf.Server.HTTPPort)
	}
	log.Info("Available on %s", conf.Server.ExternalURL)

	switch conf.Server.Protocol {
	case "http":
		err = http.ListenAndServe(listenAddr, f)

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
			}, Handler: f,
		}
		err = server.ListenAndServeTLS(conf.Server.CertFile, conf.Server.KeyFile)

	case "fcgi":
		err = fcgi.Serve(nil, f)

	case "unix":
		if osutil.Exist(listenAddr) {
			err = os.Remove(listenAddr)
			if err != nil {
				log.Fatal("Failed to remove existing Unix domain socket: %v", err)
			}
		}

		var listener *net.UnixListener
		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: listenAddr, Net: "unix"})
		if err != nil {
			log.Fatal("Failed to listen on Unix networks: %v", err)
		}

		// FIXME: add proper implementation of signal capture on all protocols
		// execute this on SIGTERM or SIGINT: listener.Close()
		if err = os.Chmod(listenAddr, conf.Server.UnixSocketMode); err != nil {
			log.Fatal("Failed to change permission of Unix domain socket: %v", err)
		}
		err = http.Serve(listener, f)

	default:
		log.Fatal("Unexpected server protocol: %s", conf.Server.Protocol)
	}

	if err != nil {
		log.Fatal("Failed to start server: %v", err)
	}

	return nil
}
