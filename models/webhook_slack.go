// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/git"
)

type SlackMeta struct {
	Channel  string `json:"channel"`
	Username string `json:"username"`
	IconURL  string `json:"icon_url"`
	Color    string `json:"color"`
}

type SlackPayload struct {
	Channel     string            `json:"channel"`
	Text        string            `json:"text"`
	Username    string            `json:"username"`
	IconURL     string            `json:"icon_url"`
	UnfurlLinks int               `json:"unfurl_links"`
	LinkNames   int               `json:"link_names"`
	Attachments []SlackAttachment `json:"attachments"`
}

type SlackAttachment struct {
	Color string `json:"color"`
	Text  string `json:"text"`
}

func (p *SlackPayload) SetSecret(_ string) {}

func (p *SlackPayload) JSONPayload() ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

// see: https://api.slack.com/docs/formatting
func SlackTextFormatter(s string) string {
	// take only first line of commit
	first := strings.Split(s, "\n")[0]
	// replace & < >
	first = strings.Replace(first, "&", "&amp;", -1)
	first = strings.Replace(first, "<", "&lt;", -1)
	first = strings.Replace(first, ">", "&gt;", -1)
	return first
}

func SlackLinkFormatter(url string, text string) string {
	return fmt.Sprintf("<%s|%s>", url, SlackTextFormatter(text))
}

func getSlackCreatePayload(p *api.CreatePayload, slack *SlackMeta) (*SlackPayload, error) {
	// created tag/branch
	refName := git.RefEndName(p.Ref)

	repoLink := SlackLinkFormatter(p.Repo.URL, p.Repo.Name)
	refLink := SlackLinkFormatter(p.Repo.URL+"/src/"+refName, refName)
	text := fmt.Sprintf("[%s:%s] %s created by %s", repoLink, refLink, p.RefType, p.Sender.UserName)

	return &SlackPayload{
		Channel:  slack.Channel,
		Text:     text,
		Username: slack.Username,
		IconURL:  slack.IconURL,
	}, nil
}

func getSlackPushPayload(p *api.PushPayload, slack *SlackMeta) (*SlackPayload, error) {
	// n new commits
	var (
		branchName   = git.RefEndName(p.Ref)
		commitString string
	)

	if len(p.Commits) == 1 {
		commitString = "1 new commit"
		if len(p.CompareUrl) > 0 {
			commitString = SlackLinkFormatter(p.CompareUrl, commitString)
		}
	} else {
		commitString = fmt.Sprintf("%d new commits", len(p.Commits))
		if p.CompareUrl != "" {
			commitString = SlackLinkFormatter(p.CompareUrl, commitString)
		}
	}

	repoLink := SlackLinkFormatter(p.Repo.URL, p.Repo.Name)
	branchLink := SlackLinkFormatter(p.Repo.URL+"/src/"+branchName, branchName)
	text := fmt.Sprintf("[%s:%s] %s pushed by %s", repoLink, branchLink, commitString, p.Pusher.Name)

	var attachmentText string
	// for each commit, generate attachment text
	for i, commit := range p.Commits {
		attachmentText += fmt.Sprintf("%s: %s - %s", SlackLinkFormatter(commit.URL, commit.ID[:7]), SlackTextFormatter(commit.Message), SlackTextFormatter(commit.Author.Name))
		// add linebreak to each commit but the last
		if i < len(p.Commits)-1 {
			attachmentText += "\n"
		}
	}

	slackAttachments := []SlackAttachment{{Color: slack.Color, Text: attachmentText}}

	return &SlackPayload{
		Channel:     slack.Channel,
		Text:        text,
		Username:    slack.Username,
		IconURL:     slack.IconURL,
		Attachments: slackAttachments,
	}, nil
}

func GetSlackPayload(p api.Payloader, event HookEventType, meta string) (*SlackPayload, error) {
	s := new(SlackPayload)

	slack := &SlackMeta{}
	if err := json.Unmarshal([]byte(meta), &slack); err != nil {
		return s, errors.New("GetSlackPayload meta json:" + err.Error())
	}

	switch event {
	case HOOK_EVENT_CREATE:
		return getSlackCreatePayload(p.(*api.CreatePayload), slack)
	case HOOK_EVENT_PUSH:
		return getSlackPushPayload(p.(*api.PushPayload), slack)
	}

	return s, nil
}
