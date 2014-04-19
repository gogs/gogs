// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"

	"github.com/codegangsta/cli"
	"github.com/go-martini/martini"
	qlog "github.com/qiniu/log"

	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/avatar"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers"
	"github.com/gogits/gogs/routers/admin"
	"github.com/gogits/gogs/routers/api/v1"
	"github.com/gogits/gogs/routers/dev"
	"github.com/gogits/gogs/routers/repo"
	"github.com/gogits/gogs/routers/user"
)

var CmdWeb = cli.Command{
	Name:  "web",
	Usage: "Gogs web server",
	Description: `
gogs web server is the only thing you need to run, 
and it takes care of all the other things for you`,
	Action: runWeb,
	Flags:  []cli.Flag{},
}

func newMartini() *martini.ClassicMartini {
	r := martini.NewRouter()
	m := martini.New()
	m.Use(middleware.Logger())
	m.Use(martini.Recovery())
	m.Use(martini.Static("public"))
	m.MapTo(r, (*martini.Routes)(nil))
	m.Action(r.Handle)
	return &martini.ClassicMartini{m, r}
}

func runWeb(*cli.Context) {
	routers.GlobalInit()

	m := newMartini()

	// Middlewares.
	m.Use(middleware.Renderer(middleware.RenderOptions{Funcs: []template.FuncMap{base.TemplateFuncs}}))
	m.Use(middleware.InitContext())

	reqSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true})
	ignSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: base.Service.RequireSignInView})
	ignSignInAndCsrf := middleware.Toggle(&middleware.ToggleOptions{DisableCsrf: true})

	reqSignOut := middleware.Toggle(&middleware.ToggleOptions{SignOutRequire: true})

	bindIgnErr := middleware.BindIgnErr

	// Routers.
	m.Get("/", ignSignIn, routers.Home)
	m.Get("/install", bindIgnErr(auth.InstallForm{}), routers.Install)
	m.Post("/install", bindIgnErr(auth.InstallForm{}), routers.InstallPost)
	m.Get("/issues", reqSignIn, user.Issues)
	m.Get("/pulls", reqSignIn, user.Pulls)
	m.Get("/stars", reqSignIn, user.Stars)
	m.Get("/help", routers.Help)

	m.Group("/api/v1", func(r martini.Router) {
		r.Post("/markdown", v1.Markdown)
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
		r.Get("/setting", user.Setting)
		r.Post("/setting", bindIgnErr(auth.UpdateProfileForm{}), user.SettingPost)
	}, reqSignIn)
	m.Group("/user", func(r martini.Router) {
		r.Get("/feeds", middleware.Bind(auth.FeedsForm{}), user.Feeds)
		r.Any("/activate", user.Activate)
		r.Get("/email2user", user.Email2User)
		r.Get("/forget_password", user.ForgotPasswd)
		r.Post("/forget_password", user.ForgotPasswdPost)
		r.Get("/logout", user.SignOut)
	})
	m.Group("/user/setting", func(r martini.Router) {
		r.Get("/social", user.SettingSocial)
		r.Get("/password", user.SettingPassword)
		r.Post("/password", bindIgnErr(auth.UpdatePasswdForm{}), user.SettingPasswordPost)
		r.Any("/ssh", bindIgnErr(auth.AddSSHKeyForm{}), user.SettingSSHKeys)
		r.Get("/notification", user.SettingNotification)
		r.Get("/security", user.SettingSecurity)
	}, reqSignIn)

	m.Get("/user/:username", ignSignIn, user.Profile)

	m.Group("/repo", func(r martini.Router) {
		m.Get("/create", repo.Create)
		m.Post("/create", bindIgnErr(auth.CreateRepoForm{}), repo.CreatePost)
		m.Get("/migrate", repo.Migrate)
		m.Post("/migrate", bindIgnErr(auth.MigrateRepoForm{}), repo.MigratePost)
	}, reqSignIn)

	adminReq := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true, AdminRequire: true})

	m.Get("/admin", adminReq, admin.Dashboard)
	m.Group("/admin", func(r martini.Router) {
		r.Get("/users", admin.Users)
		r.Get("/repos", admin.Repositories)
		r.Get("/config", admin.Config)
	}, adminReq)
	m.Group("/admin/users", func(r martini.Router) {
		r.Get("/new", admin.NewUser)
		r.Post("/new", bindIgnErr(auth.RegisterForm{}), admin.NewUserPost)
		r.Get("/:userid", admin.EditUser)
		r.Post("/:userid", bindIgnErr(auth.AdminEditUserForm{}), admin.EditUserPost)
		r.Get("/:userid/delete", admin.DeleteUser)
	}, adminReq)

	if martini.Env == martini.Dev {
		m.Get("/template/**", dev.TemplatePreview)
	}

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Get("/settings", repo.Setting)
		r.Post("/settings", repo.SettingPost)
		r.Get("/action/:action", repo.Action)
		r.Get("/issues/new", repo.CreateIssue)
		r.Post("/issues/new", bindIgnErr(auth.CreateIssueForm{}), repo.CreateIssuePost)
		r.Post("/issues/:index", bindIgnErr(auth.CreateIssueForm{}), repo.UpdateIssue)
		r.Post("/comment/:action", repo.Comment)
		r.Get("/releases/new", repo.ReleasesNew)
	}, reqSignIn, middleware.RepoAssignment(true))

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Post("/releases/new", bindIgnErr(auth.NewReleaseForm{}), repo.ReleasesNewPost)
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
		r.Get("/commit/:branchname", repo.Diff)
		r.Get("/commit/:branchname/**", repo.Diff)
		r.Get("/releases", repo.Releases)
		r.Get("/archive/:branchname/:reponame.zip", repo.ZipDownload)
	}, ignSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username", func(r martini.Router) {
		r.Get("/:reponame", middleware.RepoAssignment(true, true, true), repo.Single)
		r.Any("/:reponame/**", repo.Http)
	}, ignSignInAndCsrf)

	// Not found handler.
	m.NotFound(routers.NotFound)

	protocol := base.Cfg.MustValue("server", "PROTOCOL", "http")
	listenAddr := fmt.Sprintf("%s:%s",
		base.Cfg.MustValue("server", "HTTP_ADDR"),
		base.Cfg.MustValue("server", "HTTP_PORT", "3000"))

	if protocol == "http" {
		log.Info("Listen: http://%s", listenAddr)
		if err := http.ListenAndServe(listenAddr, m); err != nil {
			qlog.Error(err.Error())
		}
	} else if protocol == "https" {
		log.Info("Listen: https://%s", listenAddr)
		if err := http.ListenAndServeTLS(listenAddr, base.Cfg.MustValue("server", "CERT_FILE"),
			base.Cfg.MustValue("server", "KEY_FILE"), m); err != nil {
			qlog.Error(err.Error())
		}
	}
}
