package user

import (
	"encoding/json"
	"net/http"
	"github.com/gogits/gogs/models"

	"code.google.com/p/goauth2/oauth"
)

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

func (s *SocialGoogle) UserInfo(token *oauth.Token) (*BasicUserInfo, error) {
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
