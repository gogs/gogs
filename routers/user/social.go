// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"code.google.com/p/goauth2/oauth"
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

type BasicUserInfo struct {
	Identity string
	Name     string
	Email    string
}

type SocialConnector interface {
	Type() int
	SetRedirectUrl(string)
	UserInfo(*oauth.Token, *url.URL) (*BasicUserInfo, error)

	AuthCodeURL(string) string
	Exchange(string) (*oauth.Token, error)
}

func extractPath(next string) string {
	n, err := url.Parse(next)
	if err != nil {
		return "/"
	}
	return n.Path
}

var (
	SocialBaseUrl = "/user/login"
	SocialMap     = make(map[string]SocialConnector)
)

// github && google && ...
func SocialSignIn(params martini.Params, ctx *middleware.Context) {
	if base.OauthService == nil || !base.OauthService.GitHub.Enabled {
		ctx.Handle(404, "social login not enabled", nil)
		return
	}
	next := extractPath(ctx.Query("next"))
	name := params["name"]
	connect, ok := SocialMap[name]
	if !ok {
		ctx.Handle(404, "social login", nil)
		return
	}
	code := ctx.Query("code")
	if code == "" {
		// redirect to social login page
		connect.SetRedirectUrl(strings.TrimSuffix(base.AppUrl, "/") + ctx.Req.URL.Host + ctx.Req.URL.Path)
		ctx.Redirect(connect.AuthCodeURL(next))
		return
	}

	// handle call back
	tk, err := connect.Exchange(code) // exchange for token
	if err != nil {
		log.Error("oauth2 handle callback error: %v", err)
		ctx.Handle(500, "exchange code error", nil)
		return
	}
	next = extractPath(ctx.Query("state"))
	log.Trace("success get token")

	ui, err := connect.UserInfo(tk, ctx.Req.URL)
	if err != nil {
		ctx.Handle(500, fmt.Sprintf("get infomation from %s error: %v", name, err), nil)
		log.Error("social connect error: %s", err)
		return
	}
	log.Info("social login: %s", ui)
	oa, err := models.GetOauth2(ui.Identity)
	switch err {
	case nil:
		ctx.Session.Set("userId", oa.User.Id)
		ctx.Session.Set("userName", oa.User.Name)
	case models.ErrOauth2RecordNotExists:
		oa = &models.Oauth2{}
		raw, _ := json.Marshal(tk) // json encode
		oa.Token = string(raw)
		oa.Uid = -1
		oa.Type = connect.Type()
		oa.Identity = ui.Identity
		log.Trace("oa: %v", oa)
		if err = models.AddOauth2(oa); err != nil {
			log.Error("add oauth2 %v", err) // 501
			return
		}
	case models.ErrOauth2NotAssociatedWithUser:
		next = "/user/sign_up"
	default:
		log.Error("other error: %v", err)
		ctx.Handle(500, err.Error(), nil)
		return
	}
	ctx.Session.Set("socialId", oa.Id)
	ctx.Session.Set("socialName", ui.Name)
	ctx.Session.Set("socialEmail", ui.Email)
	log.Trace("socialId: %v", oa.Id)
	ctx.Redirect(next)
}

//   ________.__  __     ___ ___      ___.
//  /  _____/|__|/  |_  /   |   \ __ _\_ |__
// /   \  ___|  \   __\/    ~    \  |  \ __ \
// \    \_\  \  ||  |  \    Y    /  |  / \_\ \
//  \______  /__||__|   \___|_  /|____/|___  /
//         \/                 \/           \/

type SocialGithub struct {
	Token *oauth.Token
	*oauth.Transport
}

func (s *SocialGithub) Type() int {
	return models.OT_GITHUB
}

func init() {
	github := &SocialGithub{}
	name := "github"
	config := &oauth.Config{
		ClientId:     "09383403ff2dc16daaa1",                                       //base.OauthService.GitHub.ClientId, // FIXME: panic when set
		ClientSecret: "0e4aa0c3630df396cdcea01a9d45cacf79925fea",                   //base.OauthService.GitHub.ClientSecret,
		RedirectURL:  strings.TrimSuffix(base.AppUrl, "/") + "/user/login/" + name, //ctx.Req.URL.RequestURI(),
		Scope:        "https://api.github.com/user",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
	}
	github.Transport = &oauth.Transport{
		Config:    config,
		Transport: http.DefaultTransport,
	}
	SocialMap[name] = github
}

func (s *SocialGithub) SetRedirectUrl(url string) {
	s.Transport.Config.RedirectURL = url
}

func (s *SocialGithub) UserInfo(token *oauth.Token, _ *url.URL) (*BasicUserInfo, error) {
	transport := &oauth.Transport{
		Token: token,
	}
	var data struct {
		Id    int    `json:"id"`
		Name  string `json:"login"`
		Email string `json:"email"`
	}
	var err error
	r, err := transport.Client().Get(s.Transport.Scope)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &BasicUserInfo{
		Identity: strconv.Itoa(data.Id),
		Name:     data.Name,
		Email:    data.Email,
	}, nil
}

//   ________                     .__
//  /  _____/  ____   ____   ____ |  |   ____
// /   \  ___ /  _ \ /  _ \ / ___\|  | _/ __ \
// \    \_\  (  <_> |  <_> ) /_/  >  |_\  ___/
//  \______  /\____/ \____/\___  /|____/\___  >
//         \/             /_____/           \/

type SocialGoogle struct {
	Token *oauth.Token
	*oauth.Transport
}

func (s *SocialGoogle) Type() int {
	return models.OT_GOOGLE
}

func init() {
	google := &SocialGoogle{}
	name := "google"
	// get client id and secret from
	// https://console.developers.google.com/project
	config := &oauth.Config{
		ClientId:     "849753812404-mpd7ilvlb8c7213qn6bre6p6djjskti9.apps.googleusercontent.com", //base.OauthService.GitHub.ClientId, // FIXME: panic when set
		ClientSecret: "VukKc4MwaJUSmiyv3D7ANVCa",                                                 //base.OauthService.GitHub.ClientSecret,
		Scope:        "https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile",
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://accounts.google.com/o/oauth2/token",
	}
	google.Transport = &oauth.Transport{
		Config:    config,
		Transport: http.DefaultTransport,
	}
	SocialMap[name] = google
}

func (s *SocialGoogle) SetRedirectUrl(url string) {
	s.Transport.Config.RedirectURL = url
}

func (s *SocialGoogle) UserInfo(token *oauth.Token, _ *url.URL) (*BasicUserInfo, error) {
	transport := &oauth.Transport{Token: token}
	var data struct {
		Id    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	var err error

	reqUrl := "https://www.googleapis.com/oauth2/v1/userinfo"
	r, err := transport.Client().Get(reqUrl)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &BasicUserInfo{
		Identity: data.Id,
		Name:     data.Name,
		Email:    data.Email,
	}, nil
}

// ________   ________
// \_____  \  \_____  \
//  /  / \  \  /  / \  \
// /   \_/.  \/   \_/.  \
// \_____\ \_/\_____\ \_/
//        \__>       \__>

type SocialQQ struct {
	Token *oauth.Token
	*oauth.Transport
	reqUrl string
}

func (s *SocialQQ) Type() int {
	return models.OT_QQ
}

func init() {
	qq := &SocialQQ{}
	name := "qq"
	config := &oauth.Config{
		ClientId:     "801497180",                        //base.OauthService.GitHub.ClientId, // FIXME: panic when set
		ClientSecret: "16cd53b8ad2e16a36fc2c8f87d9388f2", //base.OauthService.GitHub.ClientSecret,
		Scope:        "all",
		AuthURL:      "https://open.t.qq.com/cgi-bin/oauth2/authorize",
		TokenURL:     "https://open.t.qq.com/cgi-bin/oauth2/access_token",
	}
	qq.reqUrl = "https://open.t.qq.com/api/user/info"
	qq.Transport = &oauth.Transport{
		Config:    config,
		Transport: http.DefaultTransport,
	}
	SocialMap[name] = qq
}

func (s *SocialQQ) SetRedirectUrl(url string) {
	s.Transport.Config.RedirectURL = url
}

func (s *SocialQQ) UserInfo(token *oauth.Token, URL *url.URL) (*BasicUserInfo, error) {
	var data struct {
		Data struct {
			Id    string `json:"openid"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"data"`
	}
	var err error
	// https://open.t.qq.com/api/user/info?
	//oauth_consumer_key=APP_KEY&
	//access_token=ACCESSTOKEN&openid=openid
	//clientip=CLIENTIP&oauth_version=2.a
	//scope=all
	var urls = url.Values{
		"oauth_consumer_key": {s.Transport.Config.ClientId},
		"access_token":       {token.AccessToken},
		"openid":             URL.Query()["openid"],
		"oauth_version":      {"2.a"},
		"scope":              {"all"},
	}
	r, err := http.Get(s.reqUrl + "?" + urls.Encode())
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &BasicUserInfo{
		Identity: data.Data.Id,
		Name:     data.Data.Name,
		Email:    data.Data.Email,
	}, nil
}
