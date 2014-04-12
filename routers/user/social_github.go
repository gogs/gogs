package user

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"code.google.com/p/goauth2/oauth"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
)

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

func (s *SocialGithub) UserInfo(token *oauth.Token) (*BasicUserInfo, error) {
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
