// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/setting"
)

type DiscordEmbedFooterObject struct {
	Text string `json:"text"`
}

type DiscordEmbedAuthorObject struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	IconURL string `json:"icon_url"`
}

type DiscordEmbedFieldObject struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type DiscordEmbedObject struct {
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	URL         string                     `json:"url"`
	Color       int                        `json:"color"`
	Footer      *DiscordEmbedFooterObject  `json:"footer"`
	Author      *DiscordEmbedAuthorObject  `json:"author"`
	Fields      []*DiscordEmbedFieldObject `json:"fields"`
}

type DiscordPayload struct {
	Content   string                `json:"content"`
	Username  string                `json:"username"`
	AvatarURL string                `json:"avatar_url"`
	Embeds    []*DiscordEmbedObject `json:"embeds"`
}

func (p *DiscordPayload) JSONPayload() ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

func DiscordTextFormatter(s string) string {
	return strings.Split(s, "\n")[0]
}

func DiscordLinkFormatter(url string, text string) string {
	return fmt.Sprintf("[%s](%s)", text, url)
}

func DiscordSHALinkFormatter(url string, text string) string {
	return fmt.Sprintf("[`%s`](%s)", text, url)
}

func getDiscordCreatePayload(p *api.CreatePayload, slack *SlackMeta) (*DiscordPayload, error) {
	// Created tag/branch
	refName := git.RefEndName(p.Ref)

	repoLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	refLink := DiscordLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName)
	content := fmt.Sprintf("Created new %s: %s/%s", p.RefType, repoLink, refLink)

	color, _ := strconv.ParseInt(strings.TrimLeft(slack.Color, "#"), 16, 32)
	return &DiscordPayload{
		Username:  slack.Username,
		AvatarURL: slack.IconURL,
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         setting.AppUrl + p.Sender.UserName,
			Color:       int(color),
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarUrl,
			},
		}},
	}, nil
}

func getDiscordPushPayload(p *api.PushPayload, slack *SlackMeta) (*DiscordPayload, error) {
	// n new commits
	var (
		branchName   = git.RefEndName(p.Ref)
		commitDesc   string
		commitString string
	)

	if len(p.Commits) == 1 {
		commitDesc = "1 new commit"
	} else {
		commitDesc = fmt.Sprintf("%d new commits", len(p.Commits))
	}

	if len(p.CompareURL) > 0 {
		commitString = DiscordLinkFormatter(p.CompareURL, commitDesc)
	} else {
		commitString = commitDesc
	}

	repoLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	branchLink := DiscordLinkFormatter(p.Repo.HTMLURL+"/src/"+branchName, branchName)
	content := fmt.Sprintf("Pushed %s to %s/%s\n", commitString, repoLink, branchLink)

	// for each commit, generate attachment text
	for i, commit := range p.Commits {
		content += fmt.Sprintf("%s %s - %s", DiscordSHALinkFormatter(commit.URL, commit.ID[:7]), DiscordTextFormatter(commit.Message), commit.Author.Name)
		// add linebreak to each commit but the last
		if i < len(p.Commits)-1 {
			content += "\n"
		}
	}

	color, _ := strconv.ParseInt(strings.TrimLeft(slack.Color, "#"), 16, 32)
	return &DiscordPayload{
		Username:  slack.Username,
		AvatarURL: slack.IconURL,
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         setting.AppUrl + p.Sender.UserName,
			Color:       int(color),
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarUrl,
			},
		}},
	}, nil
}

func getDiscordPullRequestPayload(p *api.PullRequestPayload, slack *SlackMeta) (*DiscordPayload, error) {
	title := fmt.Sprintf("#%d %s", p.Index, p.PullRequest.Title)
	url := fmt.Sprintf("%s/pulls/%d", p.Repository.HTMLURL, p.Index)
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case api.HOOK_ISSUE_OPENED:
		title = "New pull request: " + title
		content = p.PullRequest.Body
	case api.HOOK_ISSUE_CLOSED:
		if p.PullRequest.HasMerged {
			title = "Pull request merged: " + title
		} else {
			title = "Pull request closed: " + title
		}
	case api.HOOK_ISSUE_REOPENED:
		title = "Pull request re-opened: " + title
	case api.HOOK_ISSUE_EDITED:
		title = "Pull request edited: " + title
		content = p.PullRequest.Body
	case api.HOOK_ISSUE_ASSIGNED:
		title = "Pull request assigned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Assignee",
			Value: p.PullRequest.Assignee.UserName,
		}}
	case api.HOOK_ISSUE_UNASSIGNED:
		title = "Pull request unassigned: " + title
	case api.HOOK_ISSUE_LABEL_UPDATED:
		title = "Pull request labels updated: " + title
		labels := make([]string, len(p.PullRequest.Labels))
		for i := range p.PullRequest.Labels {
			labels[i] = p.PullRequest.Labels[i].Name
		}
		fields = []*DiscordEmbedFieldObject{{
			Name:  "Labels",
			Value: strings.Join(labels, ", "),
		}}
	case api.HOOK_ISSUE_LABEL_CLEARED:
		title = "Pull request labels cleared: " + title
	case api.HOOK_ISSUE_SYNCHRONIZED:
		title = "Pull request synchronized: " + title
	}

	color, _ := strconv.ParseInt(strings.TrimLeft(slack.Color, "#"), 16, 32)
	return &DiscordPayload{
		Username:  slack.Username,
		AvatarURL: slack.IconURL,
		Embeds: []*DiscordEmbedObject{{
			Title:       title,
			Description: content,
			URL:         url,
			Color:       int(color),
			Footer: &DiscordEmbedFooterObject{
				Text: p.Repository.FullName,
			},
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarUrl,
			},
			Fields: fields,
		}},
	}, nil
}

func GetDiscordPayload(p api.Payloader, event HookEventType, meta string) (*DiscordPayload, error) {
	d := new(DiscordPayload)

	slack := &SlackMeta{}
	if err := json.Unmarshal([]byte(meta), &slack); err != nil {
		return d, fmt.Errorf("GetDiscordPayload meta json: %v", err)
	}

	switch event {
	case HOOK_EVENT_CREATE:
		return getDiscordCreatePayload(p.(*api.CreatePayload), slack)
	case HOOK_EVENT_PUSH:
		return getDiscordPushPayload(p.(*api.PushPayload), slack)
	case HOOK_EVENT_PULL_REQUEST:
		return getDiscordPullRequestPayload(p.(*api.PullRequestPayload), slack)
	}

	return d, nil
}
