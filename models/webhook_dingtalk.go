// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"
)

const (
	DingtalkNotificationTitle = "Gogs Notification"
)

//Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkActionCard struct {
	Title          string `json:"title"`
	Text           string `json:"text"`
	HideAvatar     string `json:"hideAvatar"`
	BtnOrientation string `json:"btnOrientation"`
	SingleTitle    string `json:"singleTitle"`
	SingleURL      string `json:"singleURL"`
}

//Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkAtObject struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

//Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkPayload struct {
	MsgType    string             `json:"msgtype"`
	At         DingtalkAtObject   `json:"at"`
	ActionCard DingtalkActionCard `json:"actionCard"`
}

func (p *DingtalkPayload) JSONPayload() ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

func NewDingtalkActionCard(singleTitle, singleURL string) DingtalkActionCard {
	return DingtalkActionCard{
		Title:       DingtalkNotificationTitle,
		SingleURL:   singleURL,
		SingleTitle: singleTitle,
	}
}

//TODO: add content
func GetDingtalkPayload(p api.Payloader, event HookEventType) (payload *DingtalkPayload, err error) {
	switch event {
	case HOOK_EVENT_CREATE:
		payload, err = getDingtalkCreatePayload(p.(*api.CreatePayload))
	case HOOK_EVENT_DELETE:
		payload, err = getDingtalkDeletePayload(p.(*api.DeletePayload))
	case HOOK_EVENT_FORK:
		payload, err = getDingtalkForkPayload(p.(*api.ForkPayload))
	case HOOK_EVENT_PUSH:
		payload, err = getDingtalkPushPayload(p.(*api.PushPayload))
	}

	if err != nil {
		return nil, fmt.Errorf("event '%s': %v", event, err)
	}

	return payload, nil
}

func getDingtalkCreatePayload(p *api.CreatePayload) (*DingtalkPayload, error) {
	refName := git.RefEndName(p.Ref)
	refType := strings.Title(p.RefType)

	actionCard := NewDingtalkActionCard("View "+refType, p.Repo.HTMLURL+"/src/"+refName)

	actionCard.Text += "# New " + refType + " Create Event"
	actionCard.Text += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- New " + refType + ": **" + MarkdownLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName) + "**"

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

func getDingtalkDeletePayload(p *api.DeletePayload) (*DingtalkPayload, error) {
	refName := git.RefEndName(p.Ref)
	refType := strings.Title(p.RefType)

	actionCard := NewDingtalkActionCard("View Repo", p.Repo.HTMLURL)

	actionCard.Text += "# " + refType + " Delete Event"
	actionCard.Text += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- " + refType + ": **" + refName + "**"

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

func getDingtalkForkPayload(p *api.ForkPayload) (*DingtalkPayload, error) {
	actionCard := NewDingtalkActionCard("View Forkee", p.Forkee.HTMLURL)

	actionCard.Text += "# Repo Fork Event"
	actionCard.Text += "\n- From Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- To Repo: **" + MarkdownLinkFormatter(p.Forkee.HTMLURL, p.Forkee.FullName) + "**"

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

func getDingtalkPushPayload(p *api.PushPayload) (*DingtalkPayload, error) {
	refName := git.RefEndName(p.Ref)

	pusher := p.Pusher.FullName
	if pusher == "" {
		pusher = p.Pusher.UserName
	}

	var detail string
	for i, commit := range p.Commits {
		msg := strings.Split(commit.Message, "\n")[0]
		commitLink := MarkdownLinkFormatter(commit.URL, commit.ID[:7])
		detail += fmt.Sprintf("> %d. %s %s - %s\n", i, commitLink, commit.Author.Name, msg)
	}

	actionCard := NewDingtalkActionCard("View Changes", p.CompareURL)

	actionCard.Text += "# Repo Push Event"
	actionCard.Text += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- Ref: **" + MarkdownLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName) + "**"
	actionCard.Text += "\n- Pusher: **" + pusher + "**"
	actionCard.Text += "\n## " + fmt.Sprintf("Total %d commits(s)", len(p.Commits))
	actionCard.Text += "\n" + detail

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

//Format link addr and title into markdown style
func MarkdownLinkFormatter(link, text string) string {
	return "[" + text + "](" + link + ")"
}
