// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/codegangsta/cli"
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/binding"

	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers"
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

func runWeb(*cli.Context) {
	log.Info("%s %s", base.AppName, base.AppVer)

	m := martini.Classic()

	// Middlewares.
	m.Use(render.Renderer(render.Options{Funcs: []template.FuncMap{base.TemplateFuncs}}))

	// TODO: should use other store because cookie store is not secure.
	store := sessions.NewCookieStore([]byte("secret123"))
	m.Use(sessions.Sessions("my_session", store))

	m.Use(middleware.InitContext())

	// Routers.
	m.Get("/", middleware.SignInRequire(false), routers.Home)
	m.Get("/issues", middleware.SignInRequire(true), user.Issues)
	m.Get("/pulls", middleware.SignInRequire(true), user.Pulls)
	m.Get("/stars", middleware.SignInRequire(true), user.Stars)
	m.Any("/user/login", middleware.SignOutRequire(), binding.BindIgnErr(auth.LogInForm{}), user.SignIn)
	m.Any("/user/logout", middleware.SignInRequire(true), user.SignOut)
	m.Any("/user/sign_up", middleware.SignOutRequire(), binding.BindIgnErr(auth.RegisterForm{}), user.SignUp)
	m.Any("/user/delete", middleware.SignInRequire(true), user.Delete)
	m.Get("/user/feeds", binding.Bind(auth.FeedsForm{}), user.Feeds)

	m.Any("/user/setting", middleware.SignInRequire(true), binding.BindIgnErr(auth.UpdateProfileForm{}), user.Setting)
	m.Any("/user/setting/password", middleware.SignInRequire(true), binding.BindIgnErr(auth.UpdatePasswdForm{}), user.SettingPassword)
	m.Any("/user/setting/ssh", middleware.SignInRequire(true), binding.BindIgnErr(auth.AddSSHKeyForm{}), user.SettingSSHKeys)
	m.Any("/user/setting/notification", middleware.SignInRequire(true), user.SettingNotification)
	m.Any("/user/setting/security", middleware.SignInRequire(true), user.SettingSecurity)

	m.Get("/user/:username", middleware.SignInRequire(false), user.Profile)

	m.Any("/repo/create", middleware.SignInRequire(true), binding.BindIgnErr(auth.CreateRepoForm{}), repo.Create)

	m.Get("/help", routers.Help)

	m.Post("/:username/:reponame/settings", middleware.SignInRequire(true), middleware.RepoAssignment(true), repo.SettingPost)
	m.Get("/:username/:reponame/settings", middleware.SignInRequire(true), middleware.RepoAssignment(true), repo.Setting)

	m.Get("/:username/:reponame/commits", middleware.SignInRequire(false), middleware.RepoAssignment(true), repo.Commits)
	m.Get("/:username/:reponame/issues", middleware.SignInRequire(false), middleware.RepoAssignment(true), repo.Issues)
	m.Get("/:username/:reponame/pulls", middleware.SignInRequire(false), middleware.RepoAssignment(true), repo.Pulls)
	m.Get("/:username/:reponame/tree/:branchname/**",
		middleware.SignInRequire(false), middleware.RepoAssignment(true), repo.Single)
	m.Get("/:username/:reponame/tree/:branchname",
		middleware.SignInRequire(false), middleware.RepoAssignment(true), repo.Single)
	m.Get("/:username/:reponame", middleware.SignInRequire(false), middleware.RepoAssignment(true), repo.Single)

	listenAddr := fmt.Sprintf("%s:%s",
		base.Cfg.MustValue("server", "HTTP_ADDR"),
		base.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	http.ListenAndServe(listenAddr, m)
}
