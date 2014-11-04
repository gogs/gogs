// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/fcgi"
	"os"
	"path"
	"strings"

	"github.com/Unknwon/macaron"
	"github.com/codegangsta/cli"
	"github.com/macaron-contrib/binding"
	"github.com/macaron-contrib/cache"
	"github.com/macaron-contrib/captcha"
	"github.com/macaron-contrib/csrf"
	"github.com/macaron-contrib/i18n"
	"github.com/macaron-contrib/session"
	"github.com/macaron-contrib/toolbox"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/auth/apiv1"
	"github.com/gogits/gogs/modules/avatar"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/routers"
	"github.com/gogits/gogs/routers/admin"
	"github.com/gogits/gogs/routers/api/v1"
	"github.com/gogits/gogs/routers/dev"
	"github.com/gogits/gogs/routers/org"
	"github.com/gogits/gogs/routers/repo"
	"github.com/gogits/gogs/routers/user"
)

var CmdWeb = cli.Command{
	Name:  "web",
	Usage: "Start Gogs web server",
	Description: `Gogs web server is the only thing you need to run,
and it takes care of all the other things for you`,
	Action: runWeb,
	Flags:  []cli.Flag{},
}

// checkVersion checks if binary matches the version of templates files.
func checkVersion() {
	// Templates.
	data, err := ioutil.ReadFile(path.Join(setting.StaticRootPath, "templates/.VERSION"))
	if err != nil {
		log.Fatal(4, "Fail to read 'templates/.VERSION': %v", err)
	}
	if string(data) != setting.AppVer {
		log.Fatal(4, "Binary and template file version does not match, did you forget to recompile?")
	}

	// Check dependency version.
	macaronVer := git.MustParseVersion(strings.Join(strings.Split(macaron.Version(), ".")[:3], "."))
	if macaronVer.LessThan(git.MustParseVersion("0.2.3")) {
		log.Fatal(4, "Package macaron version is too old, did you forget to update?(github.com/Unknwon/macaron)")
	}
	i18nVer := git.MustParseVersion(i18n.Version())
	if i18nVer.LessThan(git.MustParseVersion("0.0.2")) {
		log.Fatal(4, "Package i18n version is too old, did you forget to update?(github.com/macaron-contrib/i18n)")
	}
	sessionVer := git.MustParseVersion(session.Version())
	if sessionVer.LessThan(git.MustParseVersion("0.0.3")) {
		log.Fatal(4, "Package session version is too old, did you forget to update?(github.com/macaron-contrib/session)")
	}
}

// newMacaron initializes Macaron instance.
func newMacaron() *macaron.Macaron {
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Use(macaron.Recovery())
	if setting.EnableGzip {
		m.Use(macaron.Gziper())
	}
	m.Use(macaron.Static(
		path.Join(setting.StaticRootPath, "public"),
		macaron.StaticOptions{
			SkipLogging: !setting.DisableRouterLog,
		},
	))
	m.Use(macaron.Renderer(macaron.RenderOptions{
		Directory:  path.Join(setting.StaticRootPath, "templates"),
		Funcs:      []template.FuncMap{base.TemplateFuncs},
		IndentJSON: macaron.Env != macaron.PROD,
	}))
	m.Use(i18n.I18n(i18n.Options{
		SubURL:          setting.AppSubUrl,
		Directory:       path.Join(setting.ConfRootPath, "locale"),
		CustomDirectory: path.Join(setting.CustomPath, "conf/locale"),
		Langs:           setting.Langs,
		Names:           setting.Names,
		Redirect:        true,
	}))
	m.Use(cache.Cacher(cache.Options{
		Adapter:  setting.CacheAdapter,
		Interval: setting.CacheInternal,
		Conn:     setting.CacheConn,
	}))
	m.Use(captcha.Captchaer(captcha.Options{
		SubURL: setting.AppSubUrl,
	}))
	m.Use(session.Sessioner(session.Options{
		Provider: setting.SessionProvider,
		Config:   *setting.SessionConfig,
	}))
	m.Use(csrf.Generate(csrf.Options{
		Secret:     setting.SecretKey,
		SetCookie:  true,
		Header:     "X-Csrf-Token",
		CookiePath: setting.AppSubUrl,
	}))
	m.Use(toolbox.Toolboxer(m, toolbox.Options{
		HealthCheckFuncs: []*toolbox.HealthCheckFuncDesc{
			&toolbox.HealthCheckFuncDesc{
				Desc: "Database connection",
				Func: models.Ping,
			},
		},
	}))
	m.Use(middleware.Contexter())
	return m
}

func runWeb(*cli.Context) {
	routers.GlobalInit()
	checkVersion()

	m := newMacaron()

	reqSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true})
	ignSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: setting.Service.RequireSignInView})
	ignSignInAndCsrf := middleware.Toggle(&middleware.ToggleOptions{DisableCsrf: true})
	reqSignOut := middleware.Toggle(&middleware.ToggleOptions{SignOutRequire: true})

	bindIgnErr := binding.BindIgnErr

	// Routers.
	m.Get("/", ignSignIn, routers.Home)
	m.Get("/explore", ignSignIn, routers.Explore)
	m.Get("/install", bindIgnErr(auth.InstallForm{}), routers.Install)
	m.Post("/install", bindIgnErr(auth.InstallForm{}), routers.InstallPost)
	m.Group("", func() {
		m.Get("/pulls", user.Pulls)
		m.Get("/issues", user.Issues)
	}, reqSignIn)

	// API routers.
	m.Group("/api", func() {
		m.Group("/v1", func() {
			// Miscellaneous.
			m.Post("/markdown", bindIgnErr(apiv1.MarkdownForm{}), v1.Markdown)
			m.Post("/markdown/raw", v1.MarkdownRaw)

			// Users.
			m.Group("/users", func() {
				m.Get("/search", v1.SearchUsers)
			})

			// Repositories.
			m.Group("/repos", func() {
				m.Get("/search", v1.SearchRepos)
				m.Post("/migrate", bindIgnErr(auth.MigrateRepoForm{}), v1.Migrate)
			})

			m.Any("/*", func(ctx *middleware.Context) {
				ctx.JSON(404, &base.ApiJsonErr{"Not Found", v1.DOC_URL})
			})
		})
	})

	// User routers.
	m.Group("/user", func() {
		m.Get("/login", user.SignIn)
		m.Post("/login", bindIgnErr(auth.SignInForm{}), user.SignInPost)
		m.Get("/login/:name", user.SocialSignIn)
		m.Get("/sign_up", user.SignUp)
		m.Post("/sign_up", bindIgnErr(auth.RegisterForm{}), user.SignUpPost)
		m.Get("/reset_password", user.ResetPasswd)
		m.Post("/reset_password", user.ResetPasswdPost)
	}, reqSignOut)
	m.Group("/user/settings", func() {
		m.Get("", user.Settings)
		m.Post("", bindIgnErr(auth.UpdateProfileForm{}), user.SettingsPost)
		m.Get("/password", user.SettingsPassword)
		m.Post("/password", bindIgnErr(auth.ChangePasswordForm{}), user.SettingsPasswordPost)
		m.Get("/ssh", user.SettingsSSHKeys)
		m.Post("/ssh", bindIgnErr(auth.AddSSHKeyForm{}), user.SettingsSSHKeysPost)
		m.Get("/social", user.SettingsSocial)
		m.Route("/delete", "GET,POST", user.SettingsDelete)
	}, reqSignIn)
	m.Group("/user", func() {
		// r.Get("/feeds", binding.Bind(auth.FeedsForm{}), user.Feeds)
		m.Any("/activate", user.Activate)
		m.Get("/email2user", user.Email2User)
		m.Get("/forget_password", user.ForgotPasswd)
		m.Post("/forget_password", user.ForgotPasswdPost)
		m.Get("/logout", user.SignOut)
	})

	// FIXME: Legacy
	m.Get("/user/:username", ignSignIn, user.Profile)

	// Gravatar service.
	avt := avatar.CacheServer("public/img/avatar/", "public/img/avatar_default.jpg")
	os.MkdirAll("public/img/avatar/", os.ModePerm)
	m.Get("/avatar/:hash", avt.ServeHTTP)

	adminReq := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true, AdminRequire: true})

	m.Group("/admin", func() {
		m.Get("", adminReq, admin.Dashboard)
		m.Get("/config", admin.Config)
		m.Get("/monitor", admin.Monitor)

		m.Group("/users", func() {
			m.Get("", admin.Users)
			m.Get("/new", admin.NewUser)
			m.Post("/new", bindIgnErr(auth.RegisterForm{}), admin.NewUserPost)
			m.Get("/:userid", admin.EditUser)
			m.Post("/:userid", bindIgnErr(auth.AdminEditUserForm{}), admin.EditUserPost)
			m.Post("/:userid/delete", admin.DeleteUser)
		})

		m.Group("/orgs", func() {
			m.Get("", admin.Organizations)
		})

		m.Group("/repos", func() {
			m.Get("", admin.Repositories)
		})

		m.Group("/auths", func() {
			m.Get("", admin.Authentications)
			m.Get("/new", admin.NewAuthSource)
			m.Post("/new", bindIgnErr(auth.AuthenticationForm{}), admin.NewAuthSourcePost)
			m.Get("/:authid", admin.EditAuthSource)
			m.Post("/:authid", bindIgnErr(auth.AuthenticationForm{}), admin.EditAuthSourcePost)
			m.Post("/:authid/delete", admin.DeleteAuthSource)
		})

		m.Group("/notices", func() {
			m.Get("", admin.Notices)
			m.Get("/:id:int/delete", admin.DeleteNotice)
		})
	}, adminReq)

	m.Get("/:username", ignSignIn, user.Profile)

	if macaron.Env == macaron.DEV {
		m.Get("/template/*", dev.TemplatePreview)
	}

	reqTrueOwner := middleware.RequireTrueOwner()

	// Organization routers.
	m.Group("/org", func() {
		m.Get("/create", org.Create)
		m.Post("/create", bindIgnErr(auth.CreateOrgForm{}), org.CreatePost)

		m.Group("/:org", func() {
			m.Get("/dashboard", user.Dashboard)
			m.Get("/members", org.Members)
			m.Get("/members/action/:action", org.MembersAction)

			m.Get("/teams", org.Teams)
			m.Get("/teams/:team", org.TeamMembers)
			m.Get("/teams/:team/repositories", org.TeamRepositories)
			m.Get("/teams/:team/action/:action", org.TeamsAction)
			m.Get("/teams/:team/action/repo/:action", org.TeamsRepoAction)
		}, middleware.OrgAssignment(true, true))

		m.Group("/:org", func() {
			m.Get("/teams/new", org.NewTeam)
			m.Post("/teams/new", bindIgnErr(auth.CreateTeamForm{}), org.NewTeamPost)
			m.Get("/teams/:team/edit", org.EditTeam)
			m.Post("/teams/:team/edit", bindIgnErr(auth.CreateTeamForm{}), org.EditTeamPost)
			m.Post("/teams/:team/delete", org.DeleteTeam)

			m.Group("/settings", func() {
				m.Get("", org.Settings)
				m.Post("", bindIgnErr(auth.UpdateOrgSettingForm{}), org.SettingsPost)
				m.Get("/hooks", org.SettingsHooks)
				m.Get("/hooks/new", repo.WebHooksNew)
				m.Post("/hooks/gogs/new", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksNewPost)
				m.Post("/hooks/slack/new", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksNewPost)
				m.Get("/hooks/:id", repo.WebHooksEdit)
				m.Post("/hooks/gogs/:id", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksEditPost)
				m.Post("/hooks/slack/:id", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksEditPost)
				m.Route("/delete", "GET,POST", org.SettingsDelete)
			})

			m.Route("/invitations/new", "GET,POST", org.Invitation)
		}, middleware.OrgAssignment(true, true, true))
	}, reqSignIn)
	m.Group("/org", func() {
		m.Get("/:org", org.Home)
	}, middleware.OrgAssignment(true))

	// Repository routers.
	m.Group("/repo", func() {
		m.Get("/create", repo.Create)
		m.Post("/create", bindIgnErr(auth.CreateRepoForm{}), repo.CreatePost)
		m.Get("/migrate", repo.Migrate)
		m.Post("/migrate", bindIgnErr(auth.MigrateRepoForm{}), repo.MigratePost)
	}, reqSignIn)

	m.Group("/:username/:reponame", func() {
		m.Get("/settings", repo.Settings)
		m.Post("/settings", bindIgnErr(auth.RepoSettingForm{}), repo.SettingsPost)
		m.Group("/settings", func() {
			m.Route("/collaboration", "GET,POST", repo.SettingsCollaboration)
			m.Get("/hooks", repo.Webhooks)
			m.Get("/hooks/new", repo.WebHooksNew)
			m.Post("/hooks/gogs/new", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksNewPost)
			m.Post("/hooks/slack/new", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksNewPost)
			m.Get("/hooks/:id", repo.WebHooksEdit)
			m.Post("/hooks/gogs/:id", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksEditPost)
			m.Post("/hooks/slack/:id", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksEditPost)

			m.Group("/hooks/git", func() {
				m.Get("", repo.GitHooks)
				m.Get("/:name", repo.GitHooksEdit)
				m.Post("/:name", repo.GitHooksEditPost)
			}, middleware.GitHookService())
		})
	}, reqSignIn, middleware.RepoAssignment(true), reqTrueOwner)

	m.Group("/:username/:reponame", func() {
		m.Get("/action/:action", repo.Action)

		m.Group("/issues", func() {
			m.Get("/new", repo.CreateIssue)
			m.Post("/new", bindIgnErr(auth.CreateIssueForm{}), repo.CreateIssuePost)
			m.Post("/:index", bindIgnErr(auth.CreateIssueForm{}), repo.UpdateIssue)
			m.Post("/:index/label", repo.UpdateIssueLabel)
			m.Post("/:index/milestone", repo.UpdateIssueMilestone)
			m.Post("/:index/assignee", repo.UpdateAssignee)
			m.Get("/:index/attachment/:id", repo.IssueGetAttachment)
			m.Post("/labels/new", bindIgnErr(auth.CreateLabelForm{}), repo.NewLabel)
			m.Post("/labels/edit", bindIgnErr(auth.CreateLabelForm{}), repo.UpdateLabel)
			m.Post("/labels/delete", repo.DeleteLabel)
			m.Get("/milestones", repo.Milestones)
			m.Get("/milestones/new", repo.NewMilestone)
			m.Post("/milestones/new", bindIgnErr(auth.CreateMilestoneForm{}), repo.NewMilestonePost)
			m.Get("/milestones/:index/edit", repo.UpdateMilestone)
			m.Post("/milestones/:index/edit", bindIgnErr(auth.CreateMilestoneForm{}), repo.UpdateMilestonePost)
			m.Get("/milestones/:index/:action", repo.UpdateMilestone)
		})

		m.Post("/comment/:action", repo.Comment)
		m.Get("/releases/new", repo.NewRelease)
		m.Get("/releases/edit/:tagname", repo.EditRelease)
	}, reqSignIn, middleware.RepoAssignment(true))

	m.Group("/:username/:reponame", func() {
		m.Post("/releases/new", bindIgnErr(auth.NewReleaseForm{}), repo.NewReleasePost)
		m.Post("/releases/edit/:tagname", bindIgnErr(auth.EditReleaseForm{}), repo.EditReleasePost)
	}, reqSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username/:reponame", func() {
		m.Get("/issues", repo.Issues)
		m.Get("/issues/:index", repo.ViewIssue)
		m.Get("/pulls", repo.Pulls)
		m.Get("/branches", repo.Branches)
		m.Get("/archive/*", repo.Download)
		m.Get("/issues2/", repo.Issues2)
	}, ignSignIn, middleware.RepoAssignment(true))

	m.Group("/:username/:reponame", func() {
		m.Get("/src/:branchname", repo.Home)
		m.Get("/src/:branchname/*", repo.Home)
		m.Get("/raw/:branchname/*", repo.SingleDownload)
		m.Get("/commits/:branchname", repo.Commits)
		m.Get("/commits/:branchname/search", repo.SearchCommits)
		m.Get("/commits/:branchname/*", repo.FileHistory)
		m.Get("/commit/:branchname", repo.Diff)
		m.Get("/commit/:branchname/*", repo.Diff)
		m.Get("/releases", repo.Releases)
		m.Get("/compare/:before([a-z0-9]+)...:after([a-z0-9]+)", repo.CompareDiff)
	}, ignSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username", func() {
		m.Get("/:reponame", ignSignIn, middleware.RepoAssignment(true, true, true), repo.Home)
		m.Any("/:reponame/*", ignSignInAndCsrf, repo.Http)
	})

	// robots.txt
	m.Get("/robots.txt", func(ctx *middleware.Context) {
		if setting.HasRobotsTxt {
			ctx.ServeFile(path.Join(setting.CustomPath, "robots.txt"))
		} else {
			ctx.Error(404)
		}
	})

	// Not found handler.
	m.NotFound(routers.NotFound)

	var err error
	listenAddr := fmt.Sprintf("%s:%s", setting.HttpAddr, setting.HttpPort)
	log.Info("Listen: %v://%s%s", setting.Protocol, listenAddr, setting.AppSubUrl)
	switch setting.Protocol {
	case setting.HTTP:
		err = http.ListenAndServe(listenAddr, m)
	case setting.HTTPS:
		err = http.ListenAndServeTLS(listenAddr, setting.CertFile, setting.KeyFile, m)
	case setting.FCGI:
		err = fcgi.Serve(nil, m)
	default:
		log.Fatal(4, "Invalid protocol: %s", setting.Protocol)
	}

	if err != nil {
		log.Fatal(4, "Fail to start server: %v", err)
	}
}
