package github

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/go-github/github"

	"gogs.io/gogs/internal/auth"
)

// Config contains configuration for GitHub authentication.
//
// ⚠️ WARNING: Change to the field name must preserve the INI key name for backward compatibility.
type Config struct {
	// the GitHub service endpoint, e.g. https://api.github.com/.
	APIEndpoint string
	SkipVerify  bool
}

func (c *Config) doAuth(login, password string) (githubLogin, fullname, email, location, website string, err error) {
	login = strings.TrimSpace(login)
	tp := github.BasicAuthTransport{
		Username: login,
		Password: strings.TrimSpace(password),
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipVerify},
		},
	}
	client, err := github.NewEnterpriseClient(c.APIEndpoint, c.APIEndpoint, tp.Client())
	if err != nil {
		return "", "", "", "", "", errors.Wrap(err, "create new client")
	}
	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return "", "", "", "", "", errors.Wrap(err, "get user info")
	}

	// GitHub ignores the Basic auth username when the password is a personal access
	// token, so the account that owns the token must be the one being signed in.
	if user.Login == nil || !strings.EqualFold(*user.Login, login) {
		return "", "", "", "", "", auth.ErrBadCredentials{Args: map[string]any{"login": login}}
	}
	githubLogin = *user.Login

	if user.Name != nil {
		fullname = *user.Name
	}
	if user.Email != nil {
		email = *user.Email
	} else {
		email = login + "+github@local"
	}
	if user.Location != nil {
		location = strings.ToUpper(*user.Location)
	}
	if user.HTMLURL != nil {
		website = strings.ToLower(*user.HTMLURL)
	}
	return githubLogin, fullname, email, location, website, nil
}
