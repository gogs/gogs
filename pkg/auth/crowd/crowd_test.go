package crowd

import "testing"

var (
	host    = "http://jira.host.cn"
	appname = "gogs"
	apppwd  = "apppwd"

	username = "username"
	userpwd  = "password"
)

func Test_CrowdAuth(t *testing.T) {
	client := NewCrowdClient(host, appname, apppwd)

	user, err := client.Auth(username, userpwd)

	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("Success: user=%v", user)
}

func Test_CrowdGetUserGroups(t *testing.T) {
	client := NewCrowdClient(host, appname, apppwd)

	groups, err := client.GetUserGroups(username)

	if err != nil {
		t.Error(err)
		return
	}

	for _, group := range groups {
		t.Logf(group.Name)
	}
}

func Test_CrowdUserInGroups(t *testing.T) {
	client := NewCrowdClient(host, appname, apppwd)

	groups := []string{
		"jira-admin",
		"jira-administrators",
	}

	not := "not "

	if client.IsUserInGroups(username, groups) {
		not = ""
	}
	t.Logf("%s is %sin %q", username, not, groups)
}
