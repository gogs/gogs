// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// api reference: http://wiki.open.t.qq.com/index.php/OAuth2.0%E9%89%B4%E6%9D%83/Authorization_code%E6%8E%88%E6%9D%83%E6%A1%88%E4%BE%8B
package user

import (
	"encoding/json"
	"net/http"
	"net/url"
	"github.com/gogits/gogs/models"

	"code.google.com/p/goauth2/oauth"
)

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
