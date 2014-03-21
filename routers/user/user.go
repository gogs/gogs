// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = "Dashboard"
	ctx.Data["PageIsUserDashboard"] = true
	repos, err := models.GetRepositories(&models.User{Id: ctx.User.Id})
	if err != nil {
		ctx.Handle(200, "user.Dashboard", err)
		return
	}
	ctx.Data["MyRepos"] = repos

	feeds, err := models.GetFeeds(ctx.User.Id, 0, false)
	if err != nil {
		ctx.Handle(200, "user.Dashboard", err)
		return
	}
	ctx.Data["Feeds"] = feeds
	ctx.HTML(200, "user/dashboard")
}

func Profile(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Profile"

	// TODO: Need to check view self or others.
	user, err := models.GetUserByName(params["username"])
	if err != nil {
		ctx.Handle(200, "user.Profile", err)
		return
	}

	ctx.Data["Owner"] = user

	tab := ctx.Query("tab")
	ctx.Data["TabName"] = tab

	switch tab {
	case "activity":
		feeds, err := models.GetFeeds(user.Id, 0, true)
		if err != nil {
			ctx.Handle(200, "user.Profile", err)
			return
		}
		ctx.Data["Feeds"] = feeds
	default:
		repos, err := models.GetRepositories(user)
		if err != nil {
			ctx.Handle(200, "user.Profile", err)
			return
		}
		ctx.Data["Repos"] = repos
	}

	ctx.Data["PageIsUserProfile"] = true
	ctx.HTML(200, "user/profile")
}

func SignIn(ctx *middleware.Context, form auth.LogInForm) {
	ctx.Data["Title"] = "Log In"

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "user/signin")
		return
	}

	if hasErr, ok := ctx.Data["HasError"]; ok && hasErr.(bool) {
		ctx.HTML(200, "user/signin")
		return
	}

	user, err := models.LoginUserPlain(form.UserName, form.Password)
	if err != nil {
		if err.Error() == models.ErrUserNotExist.Error() {
			ctx.RenderWithErr("Username or password is not correct", "user/signin", &form)
			return
		}

		ctx.Handle(200, "user.SignIn", err)
		return
	}

	ctx.Session.Set("userId", user.Id)
	ctx.Session.Set("userName", user.Name)
	ctx.Redirect("/")
}

func SignOut(ctx *middleware.Context) {
	ctx.Session.Delete("userId")
	ctx.Session.Delete("userName")
	ctx.Redirect("/")
}

func SignUp(ctx *middleware.Context, form auth.RegisterForm) {
	ctx.Data["Title"] = "Sign Up"
	ctx.Data["PageIsSignUp"] = true

	if base.Service.DisenableRegisteration {
		ctx.Data["DisenableRegisteration"] = true
		ctx.HTML(200, "user/signup")
		return
	}

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "user/signup")
		return
	}

	if form.Password != form.RetypePasswd {
		ctx.Data["HasError"] = true
		ctx.Data["Err_Password"] = true
		ctx.Data["Err_RetypePasswd"] = true
		ctx.Data["ErrorMsg"] = "Password and re-type password are not same"
		auth.AssignForm(form, ctx.Data)
	}

	if ctx.HasError() {
		ctx.HTML(200, "user/signup")
		return
	}

	u := &models.User{
		Name:     form.UserName,
		Email:    form.Email,
		Passwd:   form.Password,
		IsActive: !base.Service.RegisterEmailConfirm,
	}

	var err error
	if u, err = models.RegisterUser(u); err != nil {
		switch err {
		case models.ErrUserAlreadyExist:
			ctx.RenderWithErr("Username has been already taken", "user/signup", &form)
		case models.ErrEmailAlreadyUsed:
			ctx.RenderWithErr("E-mail address has been already used", "user/signup", &form)
		case models.ErrUserNameIllegal:
			ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), "user/signup", &form)
		default:
			ctx.Handle(200, "user.SignUp", err)
		}
		return
	}

	log.Trace("%s User created: %s", ctx.Req.RequestURI, strings.ToLower(form.UserName))

	// Send confirmation e-mail.
	if base.Service.RegisterEmailConfirm && u.Id > 1 {
		mailer.SendRegisterMail(ctx.Render, u)
		ctx.Data["IsSendRegisterMail"] = true
		ctx.Data["Email"] = u.Email
		ctx.Data["Hours"] = base.Service.ActiveCodeLives / 60
		ctx.HTML(200, "user/active")
		return
	}
	ctx.Redirect("/user/login")
}

func Delete(ctx *middleware.Context) {
	ctx.Data["Title"] = "Delete Account"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingDelete"] = true

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "user/delete")
		return
	}

	tmpUser := models.User{Passwd: ctx.Query("password")}
	tmpUser.EncodePasswd()
	if len(tmpUser.Passwd) == 0 || tmpUser.Passwd != ctx.User.Passwd {
		ctx.Data["HasError"] = true
		ctx.Data["ErrorMsg"] = "Password is not correct. Make sure you are owner of this account."
	} else {
		if err := models.DeleteUser(ctx.User); err != nil {
			ctx.Data["HasError"] = true
			switch err {
			case models.ErrUserOwnRepos:
				ctx.Data["ErrorMsg"] = "Your account still have ownership of repository, you have to delete or transfer them first."
			default:
				ctx.Handle(200, "user.Delete", err)
				return
			}
		} else {
			ctx.Redirect("/")
			return
		}
	}

	ctx.HTML(200, "user/delete")
}

const (
	TPL_FEED = `<i class="icon fa fa-%s"></i>
                        <div class="info"><span class="meta">%s</span><br>%s</div>`
)

func Feeds(ctx *middleware.Context, form auth.FeedsForm) {
	actions, err := models.GetFeeds(form.UserId, form.Page*20, false)
	if err != nil {
		ctx.JSON(500, err)
	}

	feeds := make([]string, len(actions))
	for i := range actions {
		feeds[i] = fmt.Sprintf(TPL_FEED, base.ActionIcon(actions[i].OpType),
			base.TimeSince(actions[i].Created), base.ActionDesc(actions[i], ctx.User.AvatarLink()))
	}
	ctx.JSON(200, &feeds)
}

func Issues(ctx *middleware.Context) {
	ctx.HTML(200, "user/issues")
}

func Pulls(ctx *middleware.Context) {
	ctx.HTML(200, "user/pulls")
}

func Stars(ctx *middleware.Context) {
	ctx.HTML(200, "user/stars")
}

func Activate(ctx *middleware.Context) {
	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Data["IsActivatePage"] = true
		if ctx.User.IsActive {
			ctx.Error(404)
			return
		}
		// Resend confirmation e-mail.
		if base.Service.RegisterEmailConfirm {
			ctx.Data["Hours"] = base.Service.ActiveCodeLives / 60
			mailer.SendActiveMail(ctx.Render, ctx.User)
		} else {
			ctx.Data["ServiceNotEnabled"] = true
		}
		ctx.HTML(200, "user/active")
		return
	}

	// Verify code.
	if user := models.VerifyUserActiveCode(code); user != nil {
		user.IsActive = true
		user.Rands = models.GetUserSalt()
		models.UpdateUser(user)

		log.Trace("%s User activated: %s", ctx.Req.RequestURI, user.LowerName)

		ctx.Session.Set("userId", user.Id)
		ctx.Session.Set("userName", user.Name)
		ctx.Redirect("/", 302)
		return
	}

	ctx.Data["IsActivateFailed"] = true
	ctx.HTML(200, "user/active")
}
