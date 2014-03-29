// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/codegangsta/martini"

	"github.com/gogits/binding"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/avatar"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers"
	"github.com/gogits/gogs/routers/admin"
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

// globalInit is for global configuration reload-able.
func globalInit() {
	base.NewConfigContext()
	mailer.NewMailerContext()
	models.LoadModelsConfig()
	models.LoadRepoConfig()
	models.NewRepoContext()
	models.NewEngine()
}

// Check run mode(Default of martini is Dev).
func checkRunMode() {
	switch base.Cfg.MustValue("", "RUN_MODE") {
	case "prod":
		martini.Env = martini.Prod
	case "test":
		martini.Env = martini.Test
	}
	log.Info("Run Mode: %s", strings.Title(martini.Env))
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
	globalInit()
	base.NewServices()
	checkRunMode()
	log.Info("%s %s", base.AppName, base.AppVer)

	m := newMartini()

	// Middlewares.
	m.Use(middleware.Renderer(middleware.RenderOptions{Funcs: []template.FuncMap{base.TemplateFuncs}}))

	m.Use(middleware.InitContext())

	reqSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true})
	ignSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: base.Service.RequireSignInView})
	reqSignOut := middleware.Toggle(&middleware.ToggleOptions{SignOutRequire: true})

	// Routers.
	m.Get("/", ignSignIn, routers.Home)
	m.Any("/install", routers.Install)
	m.Get("/issues", reqSignIn, user.Issues)
	m.Get("/pulls", reqSignIn, user.Pulls)
	m.Get("/stars", reqSignIn, user.Stars)
	m.Get("/help", routers.Help)

	avt := avatar.CacheServer("public/img/avatar/", "public/img/avatar_default.jpg")
	m.Get("/avatar/:hash", avt.ServeHTTP)

	m.Group("/user", func(r martini.Router) {
		r.Any("/login", binding.BindIgnErr(auth.LogInForm{}), user.SignIn)
		r.Any("/sign_up", binding.BindIgnErr(auth.RegisterForm{}), user.SignUp)
	}, reqSignOut)
	m.Group("/user", func(r martini.Router) {
		r.Any("/logout", user.SignOut)
		r.Any("/delete", user.Delete)
		r.Any("/setting", binding.BindIgnErr(auth.UpdateProfileForm{}), user.Setting)
	}, reqSignIn)
	m.Group("/user", func(r martini.Router) {
		r.Get("/feeds", binding.Bind(auth.FeedsForm{}), user.Feeds)
		r.Get("/activate", user.Activate)
	})

	m.Group("/user/setting", func(r martini.Router) {
		r.Any("/password", binding.BindIgnErr(auth.UpdatePasswdForm{}), user.SettingPassword)
		r.Any("/ssh", binding.BindIgnErr(auth.AddSSHKeyForm{}), user.SettingSSHKeys)
		r.Any("/notification", user.SettingNotification)
		r.Any("/security", user.SettingSecurity)
	}, reqSignIn)

	m.Get("/user/:username", ignSignIn, user.Profile)

	m.Any("/repo/create", reqSignIn, binding.BindIgnErr(auth.CreateRepoForm{}), repo.Create)

	adminReq := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true, AdminRequire: true})

	m.Get("/admin", adminReq, admin.Dashboard)
	m.Group("/admin", func(r martini.Router) {
		r.Get("/users", admin.Users)
		r.Get("/repos", admin.Repositories)
		r.Get("/config", admin.Config)
	}, adminReq)
	m.Group("/admin/users", func(r martini.Router) {
		r.Any("/new", binding.BindIgnErr(auth.RegisterForm{}), admin.NewUser)
		r.Any("/:userid", binding.BindIgnErr(auth.AdminEditUserForm{}), admin.EditUser)
		r.Any("/:userid/delete", admin.DeleteUser)
	}, adminReq)

	if martini.Env == martini.Dev {
		m.Get("/template/**", dev.TemplatePreview)
	}

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Post("/settings", repo.SettingPost)
		r.Get("/settings", repo.Setting)
		r.Get("/action/:action", repo.Action)
		r.Any("/issues/new", binding.BindIgnErr(auth.CreateIssueForm{}), repo.CreateIssue)
		r.Post("/issues/:index", binding.BindIgnErr(auth.CreateIssueForm{}), repo.UpdateIssue)
		r.Post("/comment/:action", repo.Comment)
	}, reqSignIn, middleware.RepoAssignment(true))
	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Get("/commits/:branchname", repo.Commits)
		r.Get("/issues", repo.Issues)
		r.Get("/issues/:index", repo.ViewIssue)
		r.Get("/pulls", repo.Pulls)
		r.Get("/branches", repo.Branches)
		r.Get("/src/:branchname", repo.Single)
		r.Get("/src/:branchname/**", repo.Single)
		r.Get("/raw/:branchname/**", repo.SingleDownload)
		r.Get("/commits/:branchname", repo.Commits)
		r.Get("/commits/:branchname", repo.Commits)
	}, ignSignIn, middleware.RepoAssignment(true))

	m.Get("/:username/:reponame/commit/:commitid/**", ignSignIn, middleware.RepoAssignment(true), repo.Diff)
	m.Get("/:username/:reponame/commit/:commitid", ignSignIn, middleware.RepoAssignment(true), repo.Diff)

	m.Group("/:username", func(r martini.Router) {
		r.Get("/:reponame", middleware.RepoAssignment(true), repo.Single)
		r.Get("/:reponame", middleware.RepoAssignment(true), repo.Single)
		r.Any("/:reponame/**", repo.Http)
	}, ignSignIn)

	// Not found handler.
	m.NotFound(routers.NotFound)

	listenAddr := fmt.Sprintf("%s:%s",
		base.Cfg.MustValue("server", "HTTP_ADDR"),
		base.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, m); err != nil {
		log.Critical(err.Error())
	}
}
