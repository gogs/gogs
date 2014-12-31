// Copyright 2014 Google Inc. All Rights Reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package social

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/macaron-contrib/oauth2"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

type BasicUserInfo struct {
	Identity string
	Name     string
	Email    string
}

type SocialConnector interface {
	Type() int
	UserInfo(*oauth2.Token, *url.URL) (*BasicUserInfo, error)
}

var (
	SocialMap = make(map[string]SocialConnector)
)

func NewOauthService() {
	if !setting.Cfg.Section("oauth").Key("ENABLED").MustBool() {
		return
	}

	oauth2.AppSubUrl = setting.AppSubUrl

	setting.OauthService = &setting.Oauther{}
	setting.OauthService.OauthInfos = make(map[string]*setting.OauthInfo)

	socialConfigs := make(map[string]*oauth2.Options)
	allOauthes := []string{"github", "google", "qq", "twitter", "weibo"}
	// Load all OAuth config data.
	for _, name := range allOauthes {
		sec := setting.Cfg.Section("oauth." + name)
		if !sec.Key("ENABLED").MustBool() {
			continue
		}
		setting.OauthService.OauthInfos[name] = &setting.OauthInfo{
			Options: oauth2.Options{
				ClientID:     sec.Key("CLIENT_ID").String(),
				ClientSecret: sec.Key("CLIENT_SECRET").String(),
				Scopes:       sec.Key("SCOPES").Strings(" "),
				PathLogin:    "/user/login/oauth2/" + name,
				PathCallback: setting.AppSubUrl + "/user/login/" + name,
				RedirectURL:  setting.AppUrl + "user/login/" + name,
			},
			AuthUrl:  sec.Key("AUTH_URL").String(),
			TokenUrl: sec.Key("TOKEN_URL").String(),
		}
		socialConfigs[name] = &oauth2.Options{
			ClientID:     setting.OauthService.OauthInfos[name].ClientID,
			ClientSecret: setting.OauthService.OauthInfos[name].ClientSecret,
			Scopes:       setting.OauthService.OauthInfos[name].Scopes,
		}
	}
	enabledOauths := make([]string, 0, 10)

	// GitHub.
	if setting.Cfg.Section("oauth.github").Key("ENABLED").MustBool() {
		setting.OauthService.GitHub = true
		newGitHubOauth(socialConfigs["github"])
		enabledOauths = append(enabledOauths, "GitHub")
	}

	// Google.
	if setting.Cfg.Section("oauth.google").Key("ENABLED").MustBool() {
		setting.OauthService.Google = true
		newGoogleOauth(socialConfigs["google"])
		enabledOauths = append(enabledOauths, "Google")
	}

	// QQ.
	if setting.Cfg.Section("oauth.qq").Key("ENABLED").MustBool() {
		setting.OauthService.Tencent = true
		newTencentOauth(socialConfigs["qq"])
		enabledOauths = append(enabledOauths, "QQ")
	}

	// Twitter.
	// if setting.Cfg.Section("oauth.twitter").Key( "ENABLED").MustBool() {
	// 	setting.OauthService.Twitter = true
	// 	newTwitterOauth(socialConfigs["twitter"])
	// 	enabledOauths = append(enabledOauths, "Twitter")
	// }

	// Weibo.
	if setting.Cfg.Section("oauth.weibo").Key("ENABLED").MustBool() {
		setting.OauthService.Weibo = true
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
	opts *oauth2.Options
}

func newGitHubOauth(opts *oauth2.Options) {
	SocialMap["github"] = &SocialGithub{opts}
}

func (s *SocialGithub) Type() int {
	return int(models.GITHUB)
}

func (s *SocialGithub) UserInfo(token *oauth2.Token, _ *url.URL) (*BasicUserInfo, error) {
	transport := s.opts.NewTransportFromToken(token)
	var data struct {
		Id    int    `json:"id"`
		Name  string `json:"login"`
		Email string `json:"email"`
	}
	r, err := transport.Client().Get("https://api.github.com/user")
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
	opts *oauth2.Options
}

func (s *SocialGoogle) Type() int {
	return int(models.GOOGLE)
}

func newGoogleOauth(opts *oauth2.Options) {
	SocialMap["google"] = &SocialGoogle{opts}
}

func (s *SocialGoogle) UserInfo(token *oauth2.Token, _ *url.URL) (*BasicUserInfo, error) {
	transport := s.opts.NewTransportFromToken(token)
	var data struct {
		Id    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	r, err := transport.Client().Get("https://www.googleapis.com/userinfo/v2/me")
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
	opts *oauth2.Options
}

func newTencentOauth(opts *oauth2.Options) {
	SocialMap["qq"] = &SocialTencent{opts}
}

func (s *SocialTencent) Type() int {
	return int(models.QQ)
}

func (s *SocialTencent) UserInfo(token *oauth2.Token, URL *url.URL) (*BasicUserInfo, error) {
	r, err := http.Get("https://graph.z.qq.com/moc2/me?access_token=" + url.QueryEscape(token.AccessToken))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}

	return &BasicUserInfo{
		Identity: vals.Get("openid"),
	}, nil
}

// ___________       .__  __    __
// \__    ___/_  _  _|__|/  |__/  |_  ___________
//   |    |  \ \/ \/ /  \   __\   __\/ __ \_  __ \
//   |    |   \     /|  ||  |  |  | \  ___/|  | \/
//   |____|    \/\_/ |__||__|  |__|  \___  >__|
//                                       \/

// type SocialTwitter struct {
// 	Token *oauth2.Token
// 	*oauth2.Transport
// }

// func (s *SocialTwitter) Type() int {
// 	return int(models.TWITTER)
// }

// func newTwitterOauth(config *oauth2.Config) {
// 	SocialMap["twitter"] = &SocialTwitter{
// 		Transport: &oauth.Transport{
// 			Config:    config,
// 			Transport: http.DefaultTransport,
// 		},
// 	}
// }

// func (s *SocialTwitter) SetRedirectUrl(url string) {
// 	s.Transport.Config.RedirectURL = url
// }

// //https://github.com/mrjones/oauth
// func (s *SocialTwitter) UserInfo(token *oauth2.Token, _ *url.URL) (*BasicUserInfo, error) {
// 	// transport := &oauth.Transport{Token: token}
// 	// var data struct {
// 	// 	Id    string `json:"id"`
// 	// 	Name  string `json:"name"`
// 	// 	Email string `json:"email"`
// 	// }
// 	// var err error

// 	// reqUrl := "https://www.googleapis.com/oauth2/v1/userinfo"
// 	// r, err := transport.Client().Get(reqUrl)
// 	// if err != nil {
// 	// 	return nil, err
// 	// }
// 	// defer r.Body.Close()
// 	// if err = json.NewDecoder(r.Body).Decode(&data); err != nil {
// 	// 	return nil, err
// 	// }
// 	// return &BasicUserInfo{
// 	// 	Identity: data.Id,
// 	// 	Name:     data.Name,
// 	// 	Email:    data.Email,
// 	// }, nil
// 	return nil, nil
// }

//  __      __       ._____.
// /  \    /  \ ____ |__\_ |__   ____
// \   \/\/   // __ \|  || __ \ /  _ \
//  \        /\  ___/|  || \_\ (  <_> )
//   \__/\  /  \___  >__||___  /\____/
//        \/       \/        \/

type SocialWeibo struct {
	opts *oauth2.Options
}

func newWeiboOauth(opts *oauth2.Options) {
	SocialMap["weibo"] = &SocialWeibo{opts}
}

func (s *SocialWeibo) Type() int {
	return int(models.WEIBO)
}

func (s *SocialWeibo) UserInfo(token *oauth2.Token, _ *url.URL) (*BasicUserInfo, error) {
	transport := s.opts.NewTransportFromToken(token)
	var data struct {
		Name string `json:"name"`
	}
	var urls = url.Values{
		"access_token": {token.AccessToken},
		"uid":          {token.Extra("uid")},
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
		Identity: token.Extra("uid"),
		Name:     data.Name,
	}, nil
}
