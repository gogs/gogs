package github

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
)

func GITHUBAuth(apiEndpoint, userName, passwd string) (string, string, string, string, string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	tp := github.BasicAuthTransport{
		Username:  strings.TrimSpace(userName),
		Password:  strings.TrimSpace(passwd),
		Transport: tr,
	}
	client, err := github.NewEnterpriseClient(apiEndpoint, apiEndpoint, tp.Client())
	if err != nil {
		return "", "", "", "", "", errors.New("Authentication failure: GitHub Api Endpoint can not be reached")
	}
	ctx := context.Background()
	user, _, err := client.Users.Get(ctx, "")
	if err != nil || user == nil {
		fmt.Println(err)
		msg := fmt.Sprintf("Authentication failure! Github Api Endpoint authticated failed! User %s", userName)
		return "", "", "", "", "", errors.New(msg)
	}

	var website = ""
	if user.HTMLURL != nil {
		website = strings.ToLower(*user.HTMLURL)
	}
	var location = ""
	if user.Location != nil {
		location = strings.ToUpper(*user.Location)
	}

	return *user.Login, *user.Name, *user.Email, website, location, nil
}
