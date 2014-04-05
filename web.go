// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/codegangsta/cli"
	"github.com/go-martini/martini"
	// "github.com/martini-contrib/oauth2"
	// "github.com/martini-contrib/sessions"

	"github.com/gogits/binding"

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
	fmt.Println("Server is running...")
	routers.GlobalInit()
	log.Info("%s %s", base.AppName, base.AppVer)

	m := newMartini()

	// Middlewares.
	m.Use(middleware.Renderer(middleware.RenderOptions{Funcs: []template.FuncMap{base.TemplateFuncs}}))

	// scope := "https://api.github.com/user"
	// oauth2.PathCallback = "/oauth2callback"
	// m.Use(sessions.Sessions("my_session", sessions.NewCookieStore([]byte("secret123"))))
	// m.Use(oauth2.Github(&oauth2.Options{
	// 	ClientId:     "09383403ff2dc16daaa1",
	// 	ClientSecret: "5f6e7101d30b77952aab22b75eadae17551ea6b5",
	// 	RedirectURL:  base.AppUrl + oauth2.PathCallback,
	// 	Scopes:       []string{scope},
	// }))

	m.Use(middleware.InitContext())

	reqSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: true})
	ignSignIn := middleware.Toggle(&middleware.ToggleOptions{SignInRequire: base.Service.RequireSignInView})
	reqSignOut := middleware.Toggle(&middleware.ToggleOptions{SignOutRequire: true})

	// Routers.
	m.Get("/", ignSignIn, routers.Home)
	m.Any("/install", binding.BindIgnErr(auth.InstallForm{}), routers.Install)
	m.Get("/issues", reqSignIn, user.Issues)
	m.Get("/pulls", reqSignIn, user.Pulls)
	m.Get("/stars", reqSignIn, user.Stars)
	m.Get("/help", routers.Help)

	m.Group("/api/v1", func(r martini.Router) {
		r.Post("/markdown", v1.Markdown)
	})

	avt := avatar.CacheServer("public/img/avatar/", "public/img/avatar_default.jpg")
	m.Get("/avatar/:hash", avt.ServeHTTP)

	m.Group("/user", func(r martini.Router) {
		r.Any("/login/github", user.SocialSignIn)
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
		r.Get("/issues", repo.Issues)
		r.Get("/issues/:index", repo.ViewIssue)
		r.Get("/releases", repo.Releases)
		r.Get("/pulls", repo.Pulls)
		r.Get("/branches", repo.Branches)
	}, ignSignIn, middleware.RepoAssignment(true))

	m.Group("/:username/:reponame", func(r martini.Router) {
		r.Get("/src/:branchname", repo.Single)
		r.Get("/src/:branchname/**", repo.Single)
		r.Get("/raw/:branchname/**", repo.SingleDownload)
		r.Get("/commits/:branchname", repo.Commits)
		r.Get("/commit/:branchname", repo.Diff)
		r.Get("/commit/:branchname/**", repo.Diff)
	}, ignSignIn, middleware.RepoAssignment(true, true))

	m.Group("/:username", func(r martini.Router) {
		r.Any("/:reponame/**", repo.Http)
		r.Get("/:reponame", middleware.RepoAssignment(true, true, true), repo.Single)
	}, ignSignIn)

	// Not found handler.
	m.NotFound(routers.NotFound)

	protocol := base.Cfg.MustValue("server", "PROTOCOL", "http")
	listenAddr := fmt.Sprintf("%s:%s",
		base.Cfg.MustValue("server", "HTTP_ADDR"),
		base.Cfg.MustValue("server", "HTTP_PORT", "3000"))

	if protocol == "http" {
		log.Info("Listen: http://%s", listenAddr)
		if err := http.ListenAndServe(listenAddr, m); err != nil {
			fmt.Println(err.Error())
			//log.Critical(err.Error()) // not working now
		}
	} else if protocol == "https" {
		log.Info("Listen: https://%s", listenAddr)
		if err := http.ListenAndServeTLS(listenAddr, base.Cfg.MustValue("server", "CERT_FILE"),
			base.Cfg.MustValue("server", "KEY_FILE"), m); err != nil {
			fmt.Println(err.Error())
		}
	}
}
