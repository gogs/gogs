// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	// "encoding/json"
	"fmt"
	"net/url"
	"strings"

	// "code.google.com/p/goauth2/oauth"
	"github.com/go-martini/martini"
	// "github.com/martini-contrib/oauth2"

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

// github && google && ...
// func SocialSignIn(tokens oauth2.Tokens) {
// 	transport := &oauth.Transport{}
// 	transport.Token = &oauth.Token{
// 		AccessToken:  tokens.Access(),
// 		RefreshToken: tokens.Refresh(),
// 		Expiry:       tokens.ExpiryTime(),
// 		Extra:        tokens.ExtraData(),
// 	}

// 	// Github API refer: https://developer.github.com/v3/users/
// 	// FIXME: need to judge url
// 	type GithubUser struct {
// 		Id    int    `json:"id"`
// 		Name  string `json:"login"`
// 		Email string `json:"email"`
// 	}

// 	// Make the request.
// 	scope := "https://api.github.com/user"
// 	r, err := transport.Client().Get(scope)
// 	if err != nil {
// 		log.Error("connect with github error: %s", err)
// 		// FIXME: handle error page
// 		return
// 	}
// 	defer r.Body.Close()

// 	user := &GithubUser{}
// 	err = json.NewDecoder(r.Body).Decode(user)
// 	if err != nil {
// 		log.Error("Get: %s", err)
// 	}
// 	log.Info("login: %s", user.Name)
// 	// FIXME: login here, user email to check auth, if not registe, then generate a uniq username
// }

func SignIn(ctx *middleware.Context, form auth.LogInForm) {
	ctx.Data["Title"] = "Log In"

	if ctx.Req.Method == "GET" {
		// Check auto-login.
		userName := ctx.GetCookie(base.CookieUserName)
		if len(userName) == 0 {
			ctx.HTML(200, "user/signin")
			return
		}

		isSucceed := false
		defer func() {
			if !isSucceed {
				log.Trace("%s auto-login cookie cleared: %s", ctx.Req.RequestURI, userName)
				ctx.SetCookie(base.CookieUserName, "", -1)
				ctx.SetCookie(base.CookieRememberName, "", -1)
			}
		}()

		user, err := models.GetUserByName(userName)
		if err != nil {
			ctx.HTML(200, "user/signin")
			return
		}

		secret := base.EncodeMd5(user.Rands + user.Passwd)
		value, _ := ctx.GetSecureCookie(secret, base.CookieRememberName)
		if value != user.Name {
			ctx.HTML(200, "user/signin")
			return
		}

		isSucceed = true
		ctx.Session.Set("userId", user.Id)
		ctx.Session.Set("userName", user.Name)
		redirectTo, _ := url.QueryUnescape(ctx.GetCookie("redirect_to"))
		if len(redirectTo) > 0 {
			ctx.SetCookie("redirect_to", "", -1)
			ctx.Redirect(redirectTo)
		} else {
			ctx.Redirect("/")
		}
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, "user/signin")
		return
	}

	user, err := models.LoginUserPlain(form.UserName, form.Password)
	if err != nil {
		if err == models.ErrUserNotExist {
			log.Trace("%s Log in failed: %s/%s", ctx.Req.RequestURI, form.UserName, form.Password)
			ctx.RenderWithErr("Username or password is not correct", "user/signin", &form)
			return
		}

		ctx.Handle(200, "user.SignIn", err)
		return
	}

	if form.Remember == "on" {
		secret := base.EncodeMd5(user.Rands + user.Passwd)
		days := 86400 * base.LogInRememberDays
		ctx.SetCookie(base.CookieUserName, user.Name, days)
		ctx.SetSecureCookie(secret, base.CookieRememberName, user.Name, days)
	}

	ctx.Session.Set("userId", user.Id)
	ctx.Session.Set("userName", user.Name)
	redirectTo, _ := url.QueryUnescape(ctx.GetCookie("redirect_to"))
	if len(redirectTo) > 0 {
		ctx.SetCookie("redirect_to", "", -1)
		ctx.Redirect(redirectTo)
	} else {
		ctx.Redirect("/")
	}
}

func SignOut(ctx *middleware.Context) {
	ctx.Session.Delete("userId")
	ctx.Session.Delete("userName")
	ctx.SetCookie(base.CookieUserName, "", -1)
	ctx.SetCookie(base.CookieRememberName, "", -1)
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

		if err = ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
			log.Error("Set cache(MailResendLimit) fail: %v", err)
		}
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
			base.TimeSince(actions[i].Created), base.ActionDesc(actions[i]))
	}
	ctx.JSON(200, &feeds)
}

func Issues(ctx *middleware.Context) {
	ctx.Data["Title"] = "Your Issues"
	ctx.Data["ViewType"] = "all"

	page, _ := base.StrTo(ctx.Query("page")).Int()
	repoId, _ := base.StrTo(ctx.Query("repoid")).Int64()

	ctx.Data["RepoId"] = repoId

	var posterId int64 = 0
	if ctx.Query("type") == "created_by" {
		posterId = ctx.User.Id
		ctx.Data["ViewType"] = "created_by"
	}

	// Get all repositories.
	repos, err := models.GetRepositories(ctx.User)
	if err != nil {
		ctx.Handle(200, "user.Issues(get repositories)", err)
		return
	}

	showRepos := make([]models.Repository, 0, len(repos))

	isShowClosed := ctx.Query("state") == "closed"
	var closedIssueCount, createdByCount, allIssueCount int

	// Get all issues.
	allIssues := make([]models.Issue, 0, 5*len(repos))
	for i, repo := range repos {
		issues, err := models.GetIssues(0, repo.Id, posterId, 0, page, isShowClosed, false, "", "")
		if err != nil {
			ctx.Handle(200, "user.Issues(get issues)", err)
			return
		}

		allIssueCount += repo.NumIssues
		closedIssueCount += repo.NumClosedIssues

		// Set repository information to issues.
		for j := range issues {
			issues[j].Repo = &repos[i]
		}
		allIssues = append(allIssues, issues...)

		repos[i].NumOpenIssues = repo.NumIssues - repo.NumClosedIssues
		if repos[i].NumOpenIssues > 0 {
			showRepos = append(showRepos, repos[i])
		}
	}

	showIssues := make([]models.Issue, 0, len(allIssues))
	ctx.Data["IsShowClosed"] = isShowClosed

	// Get posters and filter issues.
	for i := range allIssues {
		u, err := models.GetUserById(allIssues[i].PosterId)
		if err != nil {
			ctx.Handle(200, "user.Issues(get poster): %v", err)
			return
		}
		allIssues[i].Poster = u
		if u.Id == ctx.User.Id {
			createdByCount++
		}

		if repoId > 0 && repoId != allIssues[i].Repo.Id {
			continue
		}

		if isShowClosed == allIssues[i].IsClosed {
			showIssues = append(showIssues, allIssues[i])
		}
	}

	ctx.Data["Repos"] = showRepos
	ctx.Data["Issues"] = showIssues
	ctx.Data["AllIssueCount"] = allIssueCount
	ctx.Data["ClosedIssueCount"] = closedIssueCount
	ctx.Data["OpenIssueCount"] = allIssueCount - closedIssueCount
	ctx.Data["CreatedByCount"] = createdByCount
	ctx.HTML(200, "issue/user")
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
			ctx.Handle(404, "user.Activate", nil)
			return
		}
		// Resend confirmation e-mail.
		if base.Service.RegisterEmailConfirm {
			if ctx.Cache.IsExist("MailResendLimit_" + ctx.User.LowerName) {
				ctx.Data["ResendLimited"] = true
			} else {
				ctx.Data["Hours"] = base.Service.ActiveCodeLives / 60
				mailer.SendActiveMail(ctx.Render, ctx.User)
			}
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
		ctx.Redirect("/")
		return
	}

	ctx.Data["IsActivateFailed"] = true
	ctx.HTML(200, "user/active")
}
