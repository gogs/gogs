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

	"github.com/codegangsta/cli"
	"github.com/go-martini/martini"

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
	"github.com/gogits/gogs/routers/debug"
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

// checkVersion checks if binary matches the version of temolate files.
func checkVersion() {
	data, err := ioutil.ReadFile(path.Join(setting.StaticRootPath, "templates/VERSION"))
	if err != nil {
		log.Fatal("Fail to read 'templates/VERSION': %v", err)
	}
	if string(data) != setting.AppVer {
		log.Fatal("Binary and template file version does not match, did you forget to recompile?")
	}
}

func newMartini() *martini.ClassicMartini {
	r := martini.NewRouter()
	m := martini.New()
	m.Use(middleware.Logger())
	m.Use(martini.Recovery())
	m.Use(middleware.Static("public",
		middleware.StaticOptions{SkipLogging: !setting.DisableRouterLog}))
	m.MapTo(r, (*martini.Routes)(nil))
	m.Action(r.Handle)
	return &martini.ClassicMartini{m, r}
}

func runWeb(*cli.Context) {
	routers.GlobalInit()
	checkVersion()

	m := newMartini()

	// Middlewares.
	m.Use(middleware.Renderer(middleware.RenderOptions{
		Directory:  path.Join(setting.StaticRootPath, "templates"),
		Funcs:      []template.FuncMap{base.TemplateFuncs},
		IndentJSON: true,
	}))
	m.Use(middleware.InitContext())

	reqSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true})
	ignSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: setting.Service.RequireSignInView})
	ignSignInAndCsrf := middleware.Toggle(&middleware.ToggleOptions{DisableCsrf: true})

	reqSignOut := middleware.Toggle(&middleware.ToggleOptions{SignOutRequire: true})

	bindIgnErr := binding.BindIgnErr

	// Routers.
	m.Get("/", ignSignIn, routers.Home)
	m.Get("/install", bindIgnErr(auth.InstallForm{}), routers.Install)
	m.Post("/install", bindIgnErr(auth.InstallForm{}), routers.InstallPost)
	m.Group("", func(r martini.Router) {
		r.Get("/issues", user.Issues)
		r.Get("/pulls", user.Pulls)
		r.Get("/stars", user.Stars)
	}, reqSignIn)

	m.Group("/api", func(_ martini.Router) {
		m.Group("/v1", func(r martini.Router) {
			// Miscellaneous.
			r.Post("/markdown", bindIgnErr(apiv1.MarkdownForm{}), v1.Markdown)
			r.Post("/markdown/raw", v1.MarkdownRaw)

			// Users.
			r.Get("/users/search", v1.SearchUser)

			r.Any("**", func(ctx *middleware.Context) {
				ctx.JSON(404, &base.ApiJsonErr{"Not Found", v1.DOC_URL})
			})
		})
	})

	avt := avatar.CacheServer("public/img/avatar/", "public/img/avatar_default.jpg")
	os.MkdirAll("public/img/avatar/", os.ModePerm)
	m.Get("/avatar/:hash", avt.ServeHTTP)

	m.Group("/user", func(r martini.Router) {
		r.Get("/login", user.SignIn)
		r.Post("/login", bindIgnErr(auth.LogInForm{}), user.SignInPost)
		r.Get("/login/:name", user.SocialSignIn)
		r.Get("/sign_up", user.SignUp)
		r.Post("/sign_up", bindIgnErr(auth.RegisterForm{}), user.SignUpPost)
		r.Get("/reset_password", user.ResetPasswd)
		r.Post("/reset_password", user.ResetPasswdPost)
	}, reqSignOut)
	m.Group("/user", func(r martini.Router) {
		r.Get("/delete", user.Delete)
		r.Post("/delete", user.DeletePost)
		r.Get("/settings", user.Setting)
		r.Post("/settings", bindIgnErr(auth.UpdateProfileForm{}), user.SettingPost)
	}, reqSignIn)
	m.Group("/user", func(r martini.Router) {
		r.Get("/feeds", binding.Bind(auth.FeedsForm{}), user.Feeds)
		r.Any("/activate", user.Activate)
		r.Get("/email2user", user.Email2User)
		r.Get("/forget_password", user.ForgotPasswd)
		r.Post("/forget_password", user.ForgotPasswdPost)
		r.Get("/logout", user.SignOut)
	})
	m.Group("/user/settings", func(r martini.Router) {
		r.Get("/social", user.SettingSocial)
		r.Get("/password", user.SettingPassword)
		r.Post("/password", bindIgnErr(auth.UpdatePasswdForm{}), user.SettingPasswordPost)
		r.Any("/ssh", bindIgnErr(auth.AddSSHKeyForm{}), user.SettingSSHKeys)
		r.Get("/notification", user.SettingNotification)
		r.Get("/security", user.SettingSecurity)
	}, reqSignIn)

	m.Get("/user/:username", ignSignIn, user.Profile)

	m.Group("/repo", func(r martini.Router) {
		r.Get("/create", repo.Create)
		r.Post("/create", bindIgnErr(auth.CreateRepoForm{}), repo.CreatePost)
		r.Get("/migrate", repo.Migrate)
		r.Post("/migrate", bindIgnErr(auth.MigrateRepoForm{}), repo.MigratePost)
	}, reqSignIn)

	adminReq := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true, AdminRequire: true})

	m.Get("/admin", adminReq, admin.Dashboard)
	m.Group("/admin", func(r martini.Router) {
		r.Get("/users", admin.Users)
		r.Get("/repos", admin.Repositories)
		r.Get("/auths", admin.Auths)
		r.Get("/config", admin.Config)
		r.Get("/monitor", admin.Monitor)
	}, adminReq)
	m.Group("/admin/users", func(r martini.Router) {
		r.Get("/new", admin.NewUser)
		r.Post("/new", bindIgnErr(auth.RegisterForm{}), admin.NewUserPost)
		r.Get("/:userid", admin.EditUser)
		r.Post("/:userid", bindIgnErr(auth.AdminEditUserForm{}), admin.EditUserPost)
		r.Get("/:userid/delete", admin.DeleteUser)
	}, adminReq)

	m.Group("/admin/auths", func(r martini.Router) {
		r.Get("/new", admin.NewAuthSource)
		r.Post("/new", bindIgnErr(auth.AuthenticationForm{}), admin.NewAuthSourcePost)
		r.Get("/:authid", admin.EditAuthSource)
		r.Post("/:authid", bindIgnErr(auth.AuthenticationForm{}), admin.EditAuthSourcePost)
		r.Get("/:authid/delete", admin.DeleteAuthSource)
	}, adminReq)

	if martini.Env == martini.Dev {
		m.Get("/template/**", dev.TemplatePreview)
	}

	reqTrueOwner := middleware.RequireTrueOwner()

	m.Group("/org", func(r martini.Router) {
		r.Get("/create", org.New)
		r.Post("/create", bindIgnErr(auth.CreateOrgForm{}), org.NewPost)
		r.Get("/:org", org.Home)
		r.Get("/:org/dashboard", org.Dashboard)
		r.Get("/:org/members", org.Members)

		r.Get("/:org/teams", org.Teams)
		r.Get("/:org/teams/new", org.NewTeam)
		r.Post("/:org/teams/new", bindIgnErr(auth.CreateTeamForm{}), org.NewTeamPost)
		r.Get("/:org/teams/:team/edit", org.EditTeam)

		r.Get("/:org/settings", org.Settings)
		r.Post("/:org/settings", bindIgnErr(auth.OrgSettingForm{}), org.SettingsPost)
		r.Post("/:org/settings/delete", org.DeletePost)
	}, reqSignIn)

	debug.RegisterRoutes(m)

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Get("/settings", repo.Setting)
		r.Post("/settings", bindIgnErr(auth.RepoSettingForm{}), repo.SettingPost)

		m.Group("/settings", func(r martini.Router) {
			r.Get("/collaboration", repo.Collaboration)
			r.Post("/collaboration", repo.CollaborationPost)
			r.Get("/hooks", repo.WebHooks)
			r.Get("/hooks/add", repo.WebHooksAdd)
			r.Post("/hooks/add", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksAddPost)
			r.Get("/hooks/:id", repo.WebHooksEdit)
			r.Post("/hooks/:id", bindIgnErr(auth.NewWebhookForm{}), repo.WebHooksEditPost)
		})
	}, reqSignIn, middleware.RepoAssignment(true), reqTrueOwner)

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Get("/action/:action", repo.Action)

		m.Group("/issues", func(r martini.Router) {
			r.Get("/new", repo.CreateIssue)
			r.Post("/new", bindIgnErr(auth.CreateIssueForm{}), repo.CreateIssuePost)
			r.Post("/:index", bindIgnErr(auth.CreateIssueForm{}), repo.UpdateIssue)
			r.Post("/:index/label", repo.UpdateIssueLabel)
			r.Post("/:index/milestone", repo.UpdateIssueMilestone)
			r.Post("/:index/assignee", repo.UpdateAssignee)
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

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Post("/releases/new", bindIgnErr(auth.NewReleaseForm{}), repo.NewReleasePost)
		r.Post("/releases/edit/:tagname", bindIgnErr(auth.EditReleaseForm{}), repo.EditReleasePost)
	}, reqSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Get("/issues", repo.Issues)
		r.Get("/issues/:index", repo.ViewIssue)
		r.Get("/pulls", repo.Pulls)
		r.Get("/branches", repo.Branches)
	}, ignSignIn, middleware.RepoAssignment(true))

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Get("/src/:branchname", repo.Single)
		r.Get("/src/:branchname/**", repo.Single)
		r.Get("/raw/:branchname/**", repo.SingleDownload)
		r.Get("/commits/:branchname", repo.Commits)
		r.Get("/commits/:branchname/search", repo.SearchCommits)
		r.Get("/commits/:branchname/**", repo.FileHistory)
		r.Get("/commit/:branchname", repo.Diff)
		r.Get("/commit/:branchname/**", repo.Diff)
		r.Get("/releases", repo.Releases)
		r.Get("/archive/:branchname/:reponame.zip", repo.ZipDownload)
		r.Get("/archive/:branchname/:reponame.tar.gz", repo.TarGzDownload)
	}, ignSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username", func(r martini.Router) {
		r.Get("/:reponame", middleware.RepoAssignment(true, true, true), repo.Single)
		r.Any("/:reponame/**", repo.Http)
	}, ignSignInAndCsrf)

	// Not found handler.
	m.NotFound(routers.NotFound)

	var err error
	listenAddr := fmt.Sprintf("%s:%s", setting.HttpAddr, setting.HttpPort)
	log.Info("Listen: %v://%s", setting.Protocol, listenAddr)
	switch setting.Protocol {
	case setting.HTTP:
		err = http.ListenAndServe(listenAddr, m)
	case setting.HTTPS:
		err = http.ListenAndServeTLS(listenAddr, setting.CertFile, setting.KeyFile, m)
	default:
		log.Fatal("Invalid protocol: %s", setting.Protocol)
	}

	if err != nil {
		log.Fatal("Fail to start server: %v", err)
	}
}
