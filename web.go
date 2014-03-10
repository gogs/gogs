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
	Flags:  []cli.Flag{
	//cli.BoolFlag{"update, u", "update pakcage(s) and dependencies if any"},
	//cli.BoolFlag{"verbose, v", "show process details"},
	},
}

var AppHelpers template.FuncMap = map[string]interface{}{
	"AppName": func() string {
		return base.Cfg.MustValue("", "APP_NAME")
	},
	"AppVer": func() string {
		return APP_VER
	},
}

func runWeb(*cli.Context) {
	log.Info("%s %s", base.Cfg.MustValue("", "APP_NAME"), APP_VER)

	m := martini.Classic()

	// Middlewares.
	m.Use(render.Renderer(render.Options{Funcs: []template.FuncMap{AppHelpers}}))
	m.Use(base.InitContext())

	// TODO: should use other store because cookie store is not secure.
	store := sessions.NewCookieStore([]byte("secret123"))
	m.Use(sessions.Sessions("my_session", store))

	// Routers.
	m.Get("/", auth.SignInRequire(false), routers.Home)
	m.Any("/user/login", auth.SignOutRequire(), binding.BindIgnErr(auth.LogInForm{}), user.SignIn)
	m.Any("/user/logout", auth.SignInRequire(true), user.SignOut)
	m.Any("/user/sign_up", auth.SignOutRequire(), binding.BindIgnErr(auth.RegisterForm{}), user.SignUp)
	m.Any("/user/delete", auth.SignInRequire(true), user.Delete)

	m.Any("/user/setting",auth.SignInRequire(true),user.Setting)
	m.Any("/user/setting/ssh",auth.SignInRequire(true),user.SettingSSHKeys)

	m.Get("/user/:username", auth.SignInRequire(false), user.Profile)

	//m.Any("/user/publickey/add", auth.SignInRequire(true), user.AddPublicKey)
	//m.Any("/user/publickey/list", auth.SignInRequire(true), user.ListPublicKey)

	m.Any("/repo/create", auth.SignInRequire(true), binding.BindIgnErr(auth.CreateRepoForm{}), repo.Create)
	m.Any("/repo/delete", auth.SignInRequire(true), repo.Delete)
	m.Any("/repo/list", auth.SignInRequire(false), repo.List)

	listenAddr := fmt.Sprintf("%s:%s",
		base.Cfg.MustValue("server", "HTTP_ADDR"),
		base.Cfg.MustValue("server", "HTTP_PORT", "3000"))
	log.Info("Listen: %s", listenAddr)
	http.ListenAndServe(listenAddr, m)
}
