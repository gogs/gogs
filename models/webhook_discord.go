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

	"github.com/gogits/gogs/pkg/setting"
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

// getDiscordCreatePayload composes Discord payload for create new branch or tag.
func getDiscordCreatePayload(p *api.CreatePayload) (*DiscordPayload, error) {
	refName := git.RefEndName(p.Ref)
	repoLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	refLink := DiscordLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName)
	content := fmt.Sprintf("Created new %s: %s/%s", p.RefType, repoLink, refLink)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         setting.AppURL + p.Sender.UserName,
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarUrl,
			},
		}},
	}, nil
}

// getDiscordDeletePayload composes Discord payload for delete a branch or tag.
func getDiscordDeletePayload(p *api.DeletePayload) (*DiscordPayload, error) {
	refName := git.RefEndName(p.Ref)
	repoLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	content := fmt.Sprintf("Deleted %s: %s/%s", p.RefType, repoLink, refName)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         setting.AppURL + p.Sender.UserName,
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarUrl,
			},
		}},
	}, nil
}

// getDiscordForkPayload composes Discord payload for forked by a repository.
func getDiscordForkPayload(p *api.ForkPayload) (*DiscordPayload, error) {
	baseLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	forkLink := DiscordLinkFormatter(p.Forkee.HTMLURL, p.Forkee.FullName)
	content := fmt.Sprintf("%s is forked to %s", baseLink, forkLink)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         setting.AppURL + p.Sender.UserName,
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
			URL:         setting.AppURL + p.Sender.UserName,
			Color:       int(color),
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarUrl,
			},
		}},
	}, nil
}

func getDiscordIssuesPayload(p *api.IssuesPayload, slack *SlackMeta) (*DiscordPayload, error) {
	title := fmt.Sprintf("#%d %s", p.Index, p.Issue.Title)
	url := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index)
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case api.HOOK_ISSUE_OPENED:
		title = "New issue: " + title
		content = p.Issue.Body
	case api.HOOK_ISSUE_CLOSED:
		title = "Issue closed: " + title
	case api.HOOK_ISSUE_REOPENED:
		title = "Issue re-opened: " + title
	case api.HOOK_ISSUE_EDITED:
		title = "Issue edited: " + title
		content = p.Issue.Body
	case api.HOOK_ISSUE_ASSIGNED:
		title = "Issue assigned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Assignee",
			Value: p.Issue.Assignee.UserName,
		}}
	case api.HOOK_ISSUE_UNASSIGNED:
		title = "Issue unassigned: " + title
	case api.HOOK_ISSUE_LABEL_UPDATED:
		title = "Issue labels updated: " + title
		labels := make([]string, len(p.Issue.Labels))
		for i := range p.Issue.Labels {
			labels[i] = p.Issue.Labels[i].Name
		}
		if len(labels) == 0 {
			labels = []string{"<empty>"}
		}
		fields = []*DiscordEmbedFieldObject{{
			Name:  "Labels",
			Value: strings.Join(labels, ", "),
		}}
	case api.HOOK_ISSUE_LABEL_CLEARED:
		title = "Issue labels cleared: " + title
	case api.HOOK_ISSUE_SYNCHRONIZED:
		title = "Issue synchronized: " + title
	case api.HOOK_ISSUE_MILESTONED:
		title = "Issue milestoned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Milestone",
			Value: p.Issue.Milestone.Title,
		}}
	case api.HOOK_ISSUE_DEMILESTONED:
		title = "Issue demilestoned: " + title
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

func getDiscordIssueCommentPayload(p *api.IssueCommentPayload, slack *SlackMeta) (*DiscordPayload, error) {
	title := fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title)
	url := fmt.Sprintf("%s/issues/%d#%s", p.Repository.HTMLURL, p.Issue.Index, CommentHashTag(p.Comment.ID))
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case api.HOOK_ISSUE_COMMENT_CREATED:
		title = "New comment: " + title
		content = p.Comment.Body
	case api.HOOK_ISSUE_COMMENT_EDITED:
		title = "Comment edited: " + title
		content = p.Comment.Body
	case api.HOOK_ISSUE_COMMENT_DELETED:
		title = "Comment deleted: " + title
		url = fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index)
		content = p.Comment.Body
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
	case api.HOOK_ISSUE_MILESTONED:
		title = "Pull request milestoned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Milestone",
			Value: p.PullRequest.Milestone.Title,
		}}
	case api.HOOK_ISSUE_DEMILESTONED:
		title = "Pull request demilestoned: " + title
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

func getDiscordReleasePayload(p *api.ReleasePayload) (*DiscordPayload, error) {
	repoLink := DiscordLinkFormatter(p.Repository.HTMLURL, p.Repository.Name)
	refLink := DiscordLinkFormatter(p.Repository.HTMLURL+"/src/"+p.Release.TagName, p.Release.TagName)
	content := fmt.Sprintf("Published new release %s of %s", refLink, repoLink)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         setting.AppURL + p.Sender.UserName,
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarUrl,
			},
		}},
	}, nil
}

func GetDiscordPayload(p api.Payloader, event HookEventType, meta string) (payload *DiscordPayload, err error) {
	slack := &SlackMeta{}
	if err := json.Unmarshal([]byte(meta), &slack); err != nil {
		return nil, fmt.Errorf("json.Unmarshal: %v", err)
	}

	switch event {
	case HOOK_EVENT_CREATE:
		payload, err = getDiscordCreatePayload(p.(*api.CreatePayload))
	case HOOK_EVENT_DELETE:
		payload, err = getDiscordDeletePayload(p.(*api.DeletePayload))
	case HOOK_EVENT_FORK:
		payload, err = getDiscordForkPayload(p.(*api.ForkPayload))
	case HOOK_EVENT_PUSH:
		payload, err = getDiscordPushPayload(p.(*api.PushPayload), slack)
	case HOOK_EVENT_ISSUES:
		payload, err = getDiscordIssuesPayload(p.(*api.IssuesPayload), slack)
	case HOOK_EVENT_ISSUE_COMMENT:
		payload, err = getDiscordIssueCommentPayload(p.(*api.IssueCommentPayload), slack)
	case HOOK_EVENT_PULL_REQUEST:
		payload, err = getDiscordPullRequestPayload(p.(*api.PullRequestPayload), slack)
	case HOOK_EVENT_RELEASE:
		payload, err = getDiscordReleasePayload(p.(*api.ReleasePayload))
	}
	if err != nil {
		return nil, fmt.Errorf("event '%s': %v", event, err)
	}

	payload.Username = slack.Username
	payload.AvatarURL = slack.IconURL
	if len(payload.Embeds) > 0 {
		color, _ := strconv.ParseInt(strings.TrimLeft(slack.Color, "#"), 16, 32)
		payload.Embeds[0].Color = int(color)
	}

	return payload, nil
}
