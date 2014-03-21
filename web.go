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
	"github.com/martini-contrib/sessions"

	"github.com/gogits/binding"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
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
	Usage: "just run",
	Description: `
gogs web`,
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

	// TODO: should use other store because cookie store is not secure.
	store := sessions.NewCookieStore([]byte("secret123"))
	m.Use(sessions.Sessions("my_session", store))

	m.Use(middleware.InitContext())

	reqSignIn := middleware.SignInRequire(true)
	ignSignIn := middleware.SignInRequire(base.Service.RequireSignInView)
	reqSignOut := middleware.SignOutRequire()
	// Routers.
	m.Get("/", ignSignIn, routers.Home)
	m.Get("/issues", reqSignIn, user.Issues)
	m.Get("/pulls", reqSignIn, user.Pulls)
	m.Get("/stars", reqSignIn, user.Stars)
	m.Any("/user/login", reqSignOut, binding.BindIgnErr(auth.LogInForm{}), user.SignIn)
	m.Any("/user/logout", reqSignIn, user.SignOut)
	m.Any("/user/sign_up", reqSignOut, binding.BindIgnErr(auth.RegisterForm{}), user.SignUp)
	m.Any("/user/delete", reqSignIn, user.Delete)
	m.Get("/user/feeds", binding.Bind(auth.FeedsForm{}), user.Feeds)
	m.Get("/user/activate", user.Activate)

	m.Any("/user/setting", reqSignIn, binding.BindIgnErr(auth.UpdateProfileForm{}), user.Setting)
	m.Any("/user/setting/password", reqSignIn, binding.BindIgnErr(auth.UpdatePasswdForm{}), user.SettingPassword)
	m.Any("/user/setting/ssh", reqSignIn, binding.BindIgnErr(auth.AddSSHKeyForm{}), user.SettingSSHKeys)
	m.Any("/user/setting/notification", reqSignIn, user.SettingNotification)
	m.Any("/user/setting/security", reqSignIn, user.SettingSecurity)

	m.Get("/user/:username", ignSignIn, user.Profile)

	m.Any("/repo/create", reqSignIn, binding.BindIgnErr(auth.CreateRepoForm{}), repo.Create)

	m.Get("/help", routers.Help)

	adminReq := middleware.AdminRequire()
	m.Get("/admin", reqSignIn, adminReq, admin.Dashboard)
	m.Get("/admin/users", reqSignIn, adminReq, admin.Users)
	m.Any("/admin/users/new", reqSignIn, adminReq, binding.BindIgnErr(auth.RegisterForm{}), admin.NewUser)
	m.Any("/admin/users/:userid", reqSignIn, adminReq, binding.BindIgnErr(auth.AdminEditUserForm{}), admin.EditUser)
	m.Get("/admin/repos", reqSignIn, adminReq, admin.Repositories)
	m.Get("/admin/config", reqSignIn, adminReq, admin.Config)

	m.Post("/:username/:reponame/settings", reqSignIn, middleware.RepoAssignment(true), repo.SettingPost)
	m.Get("/:username/:reponame/settings", reqSignIn, middleware.RepoAssignment(true), repo.Setting)

	m.Get("/:username/:reponame/commits/:branchname", ignSignIn, middleware.RepoAssignment(true), repo.Commits)
	m.Get("/:username/:reponame/issues", ignSignIn, middleware.RepoAssignment(true), repo.Issues)
	m.Get("/:username/:reponame/pulls", ignSignIn, middleware.RepoAssignment(true), repo.Pulls)
	m.Get("/:username/:reponame/branches", ignSignIn, middleware.RepoAssignment(true), repo.Branches)
	m.Get("/:username/:reponame/action/:action", reqSignIn, middleware.RepoAssignment(true), repo.Action)
	m.Get("/:username/:reponame/src/:branchname/**",
		ignSignIn, middleware.RepoAssignment(true), repo.Single)
	m.Get("/:username/:reponame/src/:branchname",
		ignSignIn, middleware.RepoAssignment(true), repo.Single)
	m.Get("/:username/:reponame/commit/:commitid/**", ignSignIn, middleware.RepoAssignment(true), repo.Single)
	m.Get("/:username/:reponame/commit/:commitid", ignSignIn, middleware.RepoAssignment(true), repo.Single)

	m.Get("/:username/:reponame", ignSignIn, middleware.RepoAssignment(true), repo.Single)

	if martini.Env == martini.Dev {
		m.Get("/template/**", dev.TemplatePreview)
	}

	listenAddr := fmt.Sprintf("%s:%s",
		base.Cfg.MustValue("server", "HTTP_ADDR"),
		base.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, m); err != nil {
		log.Critical(err.Error())
	}
}
