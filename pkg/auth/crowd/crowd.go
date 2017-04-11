package crowd

/*
refto: https://developer.atlassian.com/display/CROWDDEV/Crowd+Developer+Documentation
*/

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type CrowdGroup struct {
	Name string `json:"name"`
}

type CrowdGroups struct {
	Groups []CrowdGroup `json:"groups"`
}

type CrowdUser struct {
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	FirstName   string `json:"first-name,omitempty"`
	LastName    string `json:"last-name,omitempty"`
	DisplayName string `json:"display-name,omitempty"`
	Email       string `json:"email,omitempty"`
}

type authRspData struct {
	CrowdUser
}

type authReqData struct {
	Value string `json:"value"`
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

type CrowdClient struct {
	host    string
	appName string
	appPwd  string
}

func (c *CrowdClient) Do(method, urlStr string, body io.Reader, v interface{}) error {

	if !strings.HasPrefix(urlStr, c.host) {
		urlStr = c.host + urlStr
	}

	req, err := http.NewRequest(method, urlStr, body)

	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", basicAuth(c.appName, c.appPwd))

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	rspBody, _ := ioutil.ReadAll(rsp.Body)
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("Status:%d, %s", rsp.StatusCode, string(rspBody))
	}

	return json.Unmarshal(rspBody, v)
}

func (c *CrowdClient) Auth(userName, password string) (user *CrowdUser, err error) {
	params := url.Values{}
	params.Add("username", userName)

	requestUri := "/rest/usermanagement/1/authentication?" + params.Encode()

	data, _ := json.Marshal(authReqData{
		Value: password,
	})

	var rspData authRspData
	err = c.Do("POST", requestUri, bytes.NewReader(data), &rspData)

	if err != nil {
		return nil, err
	}

	return &CrowdUser{
		Name:        rspData.Name,
		Active:      rspData.Active,
		FirstName:   rspData.FirstName,
		LastName:    rspData.LastName,
		DisplayName: rspData.DisplayName,
		Email:       rspData.Email,
	}, nil
}

func (c *CrowdClient) GetUserGroups(userName string) ([]CrowdGroup, error) {
	params := url.Values{}
	params.Add("username", userName)

	requestUri := "/rest/usermanagement/1/user/group/direct?" + params.Encode()

	var groups CrowdGroups
	err := c.Do("GET", requestUri, nil, &groups)

	if err != nil {
		return nil, err
	}

	return groups.Groups, nil
}

func (c *CrowdClient) IsUserInGroups(userName string, groups []string) bool {
	userGroups, err := c.GetUserGroups(userName)

	if err != nil {
		return false
	}

	for _, group := range groups {
		for _, userGroup := range userGroups {
			if userGroup.Name == group {
				return true
			}
		}
	}

	return false
}

func NewCrowdClient(host, appname, apppwd string) *CrowdClient {
	return &CrowdClient{
		host:    host,
		appName: appname,
		appPwd:  apppwd,
	}
}
