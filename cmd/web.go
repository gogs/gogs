// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"github.com/Unknwon/macaron"
	"github.com/codegangsta/cli"
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
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/middleware/binding"
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
	data, err := ioutil.ReadFile(path.Join(setting.StaticRootPath, "templates/.VERSION"))
	if err != nil {
		log.Fatal(4, "Fail to read 'templates/.VERSION': %v", err)
	}
	if string(data) != setting.AppVer {
		log.Fatal(4, "Binary and template file version does not match, did you forget to recompile?")
	}
}

// newMacaron initializes Macaron instance.
func newMacaron() *macaron.Macaron {
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Use(macaron.Recovery())
	m.Use(macaron.Static(
		path.Join(setting.StaticRootPath, "public"),
		macaron.StaticOptions{
			SkipLogging: !setting.DisableRouterLog,
		},
	))
	// if setting.EnableGzip {
	// 	m.Use(macaron.Gzip())
	// }
	m.Use(macaron.Renderer(macaron.RenderOptions{
		Directory:  path.Join(setting.StaticRootPath, "templates"),
		Funcs:      []template.FuncMap{base.TemplateFuncs},
		IndentJSON: macaron.Env != macaron.PROD,
	}))
	m.Use(i18n.I18n(i18n.Options{
		SubURL:   setting.AppSubUrl,
		Langs:    setting.Langs,
		Names:    setting.Names,
		Redirect: true,
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
	m.Group("", func(r *macaron.Router) {
		r.Get("/pulls", user.Pulls)
		r.Get("/issues", user.Issues)
	}, reqSignIn)

	// API routers.
	m.Group("/api", func(_ *macaron.Router) {
		m.Group("/v1", func(r *macaron.Router) {
			// Miscellaneous.
			r.Post("/markdown", bindIgnErr(apiv1.MarkdownForm{}), v1.Markdown)
			r.Post("/markdown/raw", v1.MarkdownRaw)

			// Users.
			m.Group("/users", func(r *macaron.Router) {
				r.Get("/search", v1.SearchUsers)
			})

			// Repositories.
			m.Group("/repos", func(r *macaron.Router) {
				r.Get("/search", v1.SearchRepos)
				r.Post("/migrate", bindIgnErr(auth.MigrateRepoForm{}), v1.Migrate)
			})

			r.Any("/*", func(ctx *middleware.Context) {
				ctx.JSON(404, &base.ApiJsonErr{"Not Found", v1.DOC_URL})
			})
		})
	})

	// User routers.
	m.Group("/user", func(r *macaron.Router) {
		r.Get("/login", user.SignIn)
		r.Post("/login", bindIgnErr(auth.SignInForm{}), user.SignInPost)
		r.Get("/login/:name", user.SocialSignIn)
		r.Get("/sign_up", user.SignUp)
		r.Post("/sign_up", bindIgnErr(auth.RegisterForm{}), user.SignUpPost)
		r.Get("/reset_password", user.ResetPasswd)
		r.Post("/reset_password", user.ResetPasswdPost)
	}, reqSignOut)
	m.Group("/user/settings", func(r *macaron.Router) {
		r.Get("", user.Settings)
		r.Post("", bindIgnErr(auth.UpdateProfileForm{}), user.SettingsPost)
		r.Get("/password", user.SettingsPassword)
		r.Post("/password", bindIgnErr(auth.ChangePasswordForm{}), user.SettingsPasswordPost)
		r.Get("/ssh", user.SettingsSSHKeys)
		r.Post("/ssh", bindIgnErr(auth.AddSSHKeyForm{}), user.SettingsSSHKeysPost)
		r.Get("/social", user.SettingsSocial)
		r.Route("/delete", "GET,POST", user.SettingsDelete)
	}, reqSignIn)
	m.Group("/user", func(r *macaron.Router) {
		// r.Get("/feeds", binding.Bind(auth.FeedsForm{}), user.Feeds)
		r.Any("/activate", user.Activate)
		r.Get("/email2user", user.Email2User)
		r.Get("/forget_password", user.ForgotPasswd)
		r.Post("/forget_password", user.ForgotPasswdPost)
		r.Get("/logout", user.SignOut)
	})

	// FIXME: Legacy
	m.Get("/user/:username", ignSignIn, user.Profile)

	// Gravatar service.
	avt := avatar.CacheServer("public/img/avatar/", "public/img/avatar_default.jpg")
	os.MkdirAll("public/img/avatar/", os.ModePerm)
	m.Get("/avatar/:hash", avt.ServeHTTP)

	adminReq := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true, AdminRequire: true})

	m.Group("/admin", func(r *macaron.Router) {
		m.Get("", adminReq, admin.Dashboard)
		r.Get("/config", admin.Config)
		r.Get("/monitor", admin.Monitor)

		m.Group("/users", func(r *macaron.Router) {
			r.Get("", admin.Users)
			r.Get("/new", admin.NewUser)
			r.Post("/new", bindIgnErr(auth.RegisterForm{}), admin.NewUserPost)
			r.Get("/:userid", admin.EditUser)
			r.Post("/:userid", bindIgnErr(auth.AdminEditUserForm{}), admin.EditUserPost)
			r.Post("/:userid/delete", admin.DeleteUser)
		})

		m.Group("/orgs", func(r *macaron.Router) {
			r.Get("", admin.Organizations)
		})

		m.Group("/repos", func(r *macaron.Router) {
			r.Get("", admin.Repositories)
		})

		m.Group("/auths", func(r *macaron.Router) {
			r.Get("", admin.Authentications)
			r.Get("/new", admin.NewAuthSource)
			r.Post("/new", bindIgnErr(auth.AuthenticationForm{}), admin.NewAuthSourcePost)
			r.Get("/:authid", admin.EditAuthSource)
			r.Post("/:authid", bindIgnErr(auth.AuthenticationForm{}), admin.EditAuthSourcePost)
			r.Post("/:authid/delete", admin.DeleteAuthSource)
		})
	}, adminReq)

	m.Get("/:username", ignSignIn, user.Profile)

	if macaron.Env == macaron.DEV {
		m.Get("/template/*", dev.TemplatePreview)
	}

	reqTrueOwner := middleware.RequireTrueOwner()

	// Organization routers.
	m.Group("/org", func(r *macaron.Router) {
		r.Get("/create", org.Create)
		r.Post("/create", bindIgnErr(auth.CreateOrgForm{}), org.CreatePost)

		m.Group("/:org", func(r *macaron.Router) {
			r.Get("/dashboard", user.Dashboard)
			r.Get("/members", org.Members)
			r.Get("/members/action/:action", org.MembersAction)

			r.Get("/teams", org.Teams)
			r.Get("/teams/:team", org.TeamMembers)
			r.Get("/teams/:team/repositories", org.TeamRepositories)
			r.Get("/teams/:team/action/:action", org.TeamsAction)
			r.Get("/teams/:team/action/repo/:action", org.TeamsRepoAction)
		}, middleware.OrgAssignment(true, true))

		m.Group("/:org", func(r *macaron.Router) {
			r.Get("/teams/new", org.NewTeam)
			r.Post("/teams/new", bindIgnErr(auth.CreateTeamForm{}), org.NewTeamPost)
			r.Get("/teams/:team/edit", org.EditTeam)
			r.Post("/teams/:team/edit", bindIgnErr(auth.CreateTeamForm{}), org.EditTeamPost)
			r.Post("/teams/:team/delete", org.DeleteTeam)

			m.Group("/settings", func(r *macaron.Router) {
				r.Get("", org.Settings)
				r.Post("", bindIgnErr(auth.UpdateOrgSettingForm{}), org.SettingsPost)
				r.Get("/hooks", org.SettingsHooks)
				r.Get("/hooks/new", repo.WebHooksNew)
				r.Post("/hooks/gogs/new", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksNewPost)
				r.Post("/hooks/slack/new", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksNewPost)
				r.Get("/hooks/:id", repo.WebHooksEdit)
				r.Post("/hooks/gogs/:id", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksEditPost)
				r.Post("/hooks/slack/:id", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksEditPost)
				r.Route("/delete", "GET,POST", org.SettingsDelete)
			})

			r.Route("/invitations/new", "GET,POST", org.Invitation)
		}, middleware.OrgAssignment(true, true, true))
	}, reqSignIn)
	m.Group("/org", func(r *macaron.Router) {
		r.Get("/:org", org.Home)
	}, middleware.OrgAssignment(true))

	// Repository routers.
	m.Group("/repo", func(r *macaron.Router) {
		r.Get("/create", repo.Create)
		r.Post("/create", bindIgnErr(auth.CreateRepoForm{}), repo.CreatePost)
		r.Get("/migrate", repo.Migrate)
		r.Post("/migrate", bindIgnErr(auth.MigrateRepoForm{}), repo.MigratePost)
	}, reqSignIn)

	m.Group("/:username/:reponame", func(r *macaron.Router) {
		r.Get("/settings", repo.Settings)
		r.Post("/settings", bindIgnErr(auth.RepoSettingForm{}), repo.SettingsPost)
		m.Group("/settings", func(r *macaron.Router) {
			r.Route("/collaboration", "GET,POST", repo.SettingsCollaboration)
			r.Get("/hooks", repo.Webhooks)
			r.Get("/hooks/new", repo.WebHooksNew)
			r.Post("/hooks/gogs/new", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksNewPost)
			r.Post("/hooks/slack/new", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksNewPost)
			r.Get("/hooks/:id", repo.WebHooksEdit)
			r.Post("/hooks/gogs/:id", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksEditPost)
			r.Post("/hooks/slack/:id", bindIgnErr(auth.NewSlackHookForm{}), repo.SlackHooksEditPost)
		})
	}, reqSignIn, middleware.RepoAssignment(true), reqTrueOwner)

	m.Group("/:username/:reponame", func(r *macaron.Router) {
		r.Get("/action/:action", repo.Action)

		m.Group("/issues", func(r *macaron.Router) {
			r.Get("/new", repo.CreateIssue)
			r.Post("/new", bindIgnErr(auth.CreateIssueForm{}), repo.CreateIssuePost)
			r.Post("/:index", bindIgnErr(auth.CreateIssueForm{}), repo.UpdateIssue)
			r.Post("/:index/label", repo.UpdateIssueLabel)
			r.Post("/:index/milestone", repo.UpdateIssueMilestone)
			r.Post("/:index/assignee", repo.UpdateAssignee)
			r.Get("/:index/attachment/:id", repo.IssueGetAttachment)
			r.Post("/labels/new", bindIgnErr(auth.CreateLabelForm{}), repo.NewLabel)
			r.Post("/labels/edit", bindIgnErr(auth.CreateLabelForm{}), repo.UpdateLabel)
			r.Post("/labels/delete", repo.DeleteLabel)
			r.Get("/milestones", repo.Milestones)
			r.Get("/milestones/new", repo.NewMilestone)
			r.Post("/milestones/new", bindIgnErr(auth.CreateMilestoneForm{}), repo.NewMilestonePost)
			r.Get("/milestones/:index/edit", repo.UpdateMilestone)
			r.Post("/milestones/:index/edit", bindIgnErr(auth.CreateMilestoneForm{}), repo.UpdateMilestonePost)
			r.Get("/milestones/:index/:action", repo.UpdateMilestone)
		})

		r.Post("/comment/:action", repo.Comment)
		r.Get("/releases/new", repo.NewRelease)
		r.Get("/releases/edit/:tagname", repo.EditRelease)
	}, reqSignIn, middleware.RepoAssignment(true))

	m.Group("/:username/:reponame", func(r *macaron.Router) {
		r.Post("/releases/new", bindIgnErr(auth.NewReleaseForm{}), repo.NewReleasePost)
		r.Post("/releases/edit/:tagname", bindIgnErr(auth.EditReleaseForm{}), repo.EditReleasePost)
	}, reqSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username/:reponame", func(r *macaron.Router) {
		r.Get("/issues", repo.Issues)
		r.Get("/issues/:index", repo.ViewIssue)
		r.Get("/pulls", repo.Pulls)
		r.Get("/branches", repo.Branches)
		r.Get("/archive/*", repo.Download)
		r.Get("/issues2/",repo.Issues2)
	}, ignSignIn, middleware.RepoAssignment(true))

	m.Group("/:username/:reponame", func(r *macaron.Router) {
		r.Get("/src/:branchname", repo.Home)
		r.Get("/src/:branchname/*", repo.Home)
		r.Get("/raw/:branchname/*", repo.SingleDownload)
		r.Get("/commits/:branchname", repo.Commits)
		r.Get("/commits/:branchname/search", repo.SearchCommits)
		r.Get("/commits/:branchname/*", repo.FileHistory)
		r.Get("/commit/:branchname", repo.Diff)
		r.Get("/commit/:branchname/*", repo.Diff)
		r.Get("/releases", repo.Releases)
		r.Get("/compare/:before([a-z0-9]+)...:after([a-z0-9]+)", repo.CompareDiff)
	}, ignSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username", func(r *macaron.Router) {
		r.Get("/:reponame", ignSignIn, middleware.RepoAssignment(true, true, true), repo.Home)
		r.Any("/:reponame/*", ignSignInAndCsrf, repo.Http)
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
	default:
		log.Fatal(4, "Invalid protocol: %s", setting.Protocol)
	}

	if err != nil {
		log.Fatal(4, "Fail to start server: %v", err)
	}
}
