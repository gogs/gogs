// Copyright 2014 Google Inc. All Rights Reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package social

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	oauth "github.com/gogits/oauth2"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
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

var (
	SocialBaseUrl = "/user/login"
	SocialMap     = make(map[string]SocialConnector)
)

func NewOauthService() {
	if !base.Cfg.MustBool("oauth", "ENABLED") {
		return
	}

	base.OauthService = &base.Oauther{}
	base.OauthService.OauthInfos = make(map[string]*base.OauthInfo)

	socialConfigs := make(map[string]*oauth.Config)
	allOauthes := []string{"github", "google", "qq", "twitter", "weibo"}
	// Load all OAuth config data.
	for _, name := range allOauthes {
		base.OauthService.OauthInfos[name] = &base.OauthInfo{
			ClientId:     base.Cfg.MustValue("oauth."+name, "CLIENT_ID"),
			ClientSecret: base.Cfg.MustValue("oauth."+name, "CLIENT_SECRET"),
			Scopes:       base.Cfg.MustValue("oauth."+name, "SCOPES"),
			AuthUrl:      base.Cfg.MustValue("oauth."+name, "AUTH_URL"),
			TokenUrl:     base.Cfg.MustValue("oauth."+name, "TOKEN_URL"),
		}
		socialConfigs[name] = &oauth.Config{
			ClientId:     base.OauthService.OauthInfos[name].ClientId,
			ClientSecret: base.OauthService.OauthInfos[name].ClientSecret,
			RedirectURL:  strings.TrimSuffix(base.AppUrl, "/") + SocialBaseUrl + name,
			Scope:        base.OauthService.OauthInfos[name].Scopes,
			AuthURL:      base.OauthService.OauthInfos[name].AuthUrl,
			TokenURL:     base.OauthService.OauthInfos[name].TokenUrl,
		}
	}

	enabledOauths := make([]string, 0, 10)

	// GitHub.
	if base.Cfg.MustBool("oauth.github", "ENABLED") {
		base.OauthService.GitHub = true
		newGitHubOauth(socialConfigs["github"])
		enabledOauths = append(enabledOauths, "GitHub")
	}

	// Google.
	if base.Cfg.MustBool("oauth.google", "ENABLED") {
		base.OauthService.Google = true
		newGoogleOauth(socialConfigs["google"])
		enabledOauths = append(enabledOauths, "Google")
	}

	// QQ.
	if base.Cfg.MustBool("oauth.qq", "ENABLED") {
		base.OauthService.Tencent = true
		newTencentOauth(socialConfigs["qq"])
		enabledOauths = append(enabledOauths, "QQ")
	}

	// Twitter.
	if base.Cfg.MustBool("oauth.twitter", "ENABLED") {
		base.OauthService.Twitter = true
		newTwitterOauth(socialConfigs["twitter"])
		enabledOauths = append(enabledOauths, "Twitter")
	}

	// Weibo.
	if base.Cfg.MustBool("oauth.weibo", "ENABLED") {
		base.OauthService.Weibo = true
		newWeiboOauth(socialConfigs["weibo"])
		enabledOauths = append(enabledOauths, "Weibo")
	}

	log.Info("Oauth Service Enabled %s", enabledOauths)
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

func newGitHubOauth(config *oauth.Config) {
	SocialMap["github"] = &SocialGithub{
		Transport: &oauth.Transport{
			Config:    config,
			Transport: http.DefaultTransport,
		},
	}
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

func newGoogleOauth(config *oauth.Config) {
	SocialMap["google"] = &SocialGoogle{
		Transport: &oauth.Transport{
			Config:    config,
			Transport: http.DefaultTransport,
		},
	}
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

type SocialTencent struct {
	Token *oauth.Token
	*oauth.Transport
	reqUrl string
}

func (s *SocialTencent) Type() int {
	return models.OT_QQ
}

func newTencentOauth(config *oauth.Config) {
	SocialMap["qq"] = &SocialTencent{
		reqUrl: "https://open.t.qq.com/api/user/info",
		Transport: &oauth.Transport{
			Config:    config,
			Transport: http.DefaultTransport,
		},
	}
}

func (s *SocialTencent) SetRedirectUrl(url string) {
	s.Transport.Config.RedirectURL = url
}

func (s *SocialTencent) UserInfo(token *oauth.Token, URL *url.URL) (*BasicUserInfo, error) {
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

// ___________       .__  __    __
// \__    ___/_  _  _|__|/  |__/  |_  ___________
//   |    |  \ \/ \/ /  \   __\   __\/ __ \_  __ \
//   |    |   \     /|  ||  |  |  | \  ___/|  | \/
//   |____|    \/\_/ |__||__|  |__|  \___  >__|
//                                       \/

type SocialTwitter struct {
	Token *oauth.Token
	*oauth.Transport
}

func (s *SocialTwitter) Type() int {
	return models.OT_TWITTER
}

func newTwitterOauth(config *oauth.Config) {
	SocialMap["twitter"] = &SocialTwitter{
		Transport: &oauth.Transport{
			Config:    config,
			Transport: http.DefaultTransport,
		},
	}
}

func (s *SocialTwitter) SetRedirectUrl(url string) {
	s.Transport.Config.RedirectURL = url
}

//https://github.com/mrjones/oauth
func (s *SocialTwitter) UserInfo(token *oauth.Token, _ *url.URL) (*BasicUserInfo, error) {
	// transport := &oauth.Transport{Token: token}
	// var data struct {
	// 	Id    string `json:"id"`
	// 	Name  string `json:"name"`
	// 	Email string `json:"email"`
	// }
	// var err error

	// reqUrl := "https://www.googleapis.com/oauth2/v1/userinfo"
	// r, err := transport.Client().Get(reqUrl)
	// if err != nil {
	// 	return nil, err
	// }
	// defer r.Body.Close()
	// if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
	// 	return nil, err
	// }
	// return &BasicUserInfo{
	// 	Identity: data.Id,
	// 	Name:     data.Name,
	// 	Email:    data.Email,
	// }, nil
	return nil, nil
}

//  __      __       ._____.
// /  \    /  \ ____ |__\_ |__   ____
// \   \/\/   // __ \|  || __ \ /  _ \
//  \        /\  ___/|  || \_\ (  <_> )
//   \__/\  /  \___  >__||___  /\____/
//        \/       \/        \/

type SocialWeibo struct {
	Token *oauth.Token
	*oauth.Transport
}

func (s *SocialWeibo) Type() int {
	return models.OT_WEIBO
}

func newWeiboOauth(config *oauth.Config) {
	SocialMap["weibo"] = &SocialWeibo{
		Transport: &oauth.Transport{
			Config:    config,
			Transport: http.DefaultTransport,
		},
	}
}

func (s *SocialWeibo) SetRedirectUrl(url string) {
	s.Transport.Config.RedirectURL = url
}

func (s *SocialWeibo) UserInfo(token *oauth.Token, _ *url.URL) (*BasicUserInfo, error) {
	transport := &oauth.Transport{Token: token}
	var data struct {
		Name string `json:"name"`
	}
	var err error

	var urls = url.Values{
		"access_token": {token.AccessToken},
		"uid":          {token.Extra["id_token"]},
	}
	reqUrl := "https://api.weibo.com/2/users/show.json"
	r, err := transport.Client().Get(reqUrl + "?" + urls.Encode())
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &BasicUserInfo{
		Identity: token.Extra["id_token"],
		Name:     data.Name,
	}, nil
	return nil, nil
}
