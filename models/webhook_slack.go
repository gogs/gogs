// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/pkg/setting"
)

type SlackMeta struct {
	Channel  string `json:"channel"`
	Username string `json:"username"`
	IconURL  string `json:"icon_url"`
	Color    string `json:"color"`
}

type SlackAttachment struct {
	Fallback string `json:"fallback"`
	Color    string `json:"color"`
	Title    string `json:"title"`
	Text     string `json:"text"`
}

type SlackPayload struct {
	Channel     string             `json:"channel"`
	Text        string             `json:"text"`
	Username    string             `json:"username"`
	IconURL     string             `json:"icon_url"`
	UnfurlLinks int                `json:"unfurl_links"`
	LinkNames   int                `json:"link_names"`
	Attachments []*SlackAttachment `json:"attachments"`
}

func (p *SlackPayload) JSONPayload() ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

// see: https://api.slack.com/docs/formatting
func SlackTextFormatter(s string) string {
	// replace & < >
	s = strings.Replace(s, "&", "&amp;", -1)
	s = strings.Replace(s, "<", "&lt;", -1)
	s = strings.Replace(s, ">", "&gt;", -1)
	return s
}

func SlackShortTextFormatter(s string) string {
	s = strings.Split(s, "\n")[0]
	// replace & < >
	s = strings.Replace(s, "&", "&amp;", -1)
	s = strings.Replace(s, "<", "&lt;", -1)
	s = strings.Replace(s, ">", "&gt;", -1)
	return s
}

func SlackLinkFormatter(url string, text string) string {
	return fmt.Sprintf("<%s|%s>", url, SlackTextFormatter(text))
}

// getSlackCreatePayload composes Slack payload for create new branch or tag.
func getSlackCreatePayload(p *api.CreatePayload) (*SlackPayload, error) {
	refName := git.RefEndName(p.Ref)
	repoLink := SlackLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	refLink := SlackLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName)
	text := fmt.Sprintf("[%s:%s] %s created by %s", repoLink, refLink, p.RefType, p.Sender.UserName)
	return &SlackPayload{
		Text: text,
	}, nil
}

// getSlackDeletePayload composes Slack payload for delete a branch or tag.
func getSlackDeletePayload(p *api.DeletePayload) (*SlackPayload, error) {
	refName := git.RefEndName(p.Ref)
	repoLink := SlackLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	text := fmt.Sprintf("[%s:%s] %s deleted by %s", repoLink, refName, p.RefType, p.Sender.UserName)
	return &SlackPayload{
		Text: text,
	}, nil
}

// getSlackForkPayload composes Slack payload for forked by a repository.
func getSlackForkPayload(p *api.ForkPayload) (*SlackPayload, error) {
	baseLink := SlackLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	forkLink := SlackLinkFormatter(p.Forkee.HTMLURL, p.Forkee.FullName)
	text := fmt.Sprintf("%s is forked to %s", baseLink, forkLink)
	return &SlackPayload{
		Text: text,
	}, nil
}

func getSlackPushPayload(p *api.PushPayload, slack *SlackMeta) (*SlackPayload, error) {
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
		commitString = SlackLinkFormatter(p.CompareURL, commitDesc)
	} else {
		commitString = commitDesc
	}

	repoLink := SlackLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	branchLink := SlackLinkFormatter(p.Repo.HTMLURL+"/src/"+branchName, branchName)
	text := fmt.Sprintf("[%s:%s] %s pushed by %s", repoLink, branchLink, commitString, p.Pusher.UserName)

	var attachmentText string
	// for each commit, generate attachment text
	for i, commit := range p.Commits {
		attachmentText += fmt.Sprintf("%s: %s - %s", SlackLinkFormatter(commit.URL, commit.ID[:7]), SlackShortTextFormatter(commit.Message), SlackTextFormatter(commit.Author.Name))
		// add linebreak to each commit but the last
		if i < len(p.Commits)-1 {
			attachmentText += "\n"
		}
	}

	return &SlackPayload{
		Channel:  slack.Channel,
		Text:     text,
		Username: slack.Username,
		IconURL:  slack.IconURL,
		Attachments: []*SlackAttachment{{
			Color: slack.Color,
			Text:  attachmentText,
		}},
	}, nil
}

func getSlackIssuesPayload(p *api.IssuesPayload, slack *SlackMeta) (*SlackPayload, error) {
	senderLink := SlackLinkFormatter(setting.AppURL+p.Sender.UserName, p.Sender.UserName)
	titleLink := SlackLinkFormatter(fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index),
		fmt.Sprintf("#%d %s", p.Index, p.Issue.Title))
	var text, title, attachmentText string
	switch p.Action {
	case api.HOOK_ISSUE_OPENED:
		text = fmt.Sprintf("[%s] New issue created by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.Issue.Body)
	case api.HOOK_ISSUE_CLOSED:
		text = fmt.Sprintf("[%s] Issue closed: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_REOPENED:
		text = fmt.Sprintf("[%s] Issue re-opened: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_EDITED:
		text = fmt.Sprintf("[%s] Issue edited: %s by %s", p.Repository.FullName, titleLink, senderLink)
		attachmentText = SlackTextFormatter(p.Issue.Body)
	case api.HOOK_ISSUE_ASSIGNED:
		text = fmt.Sprintf("[%s] Issue assigned to %s: %s by %s", p.Repository.FullName,
			SlackLinkFormatter(setting.AppURL+p.Issue.Assignee.UserName, p.Issue.Assignee.UserName),
			titleLink, senderLink)
	case api.HOOK_ISSUE_UNASSIGNED:
		text = fmt.Sprintf("[%s] Issue unassigned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_LABEL_UPDATED:
		text = fmt.Sprintf("[%s] Issue labels updated: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_LABEL_CLEARED:
		text = fmt.Sprintf("[%s] Issue labels cleared: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_MILESTONED:
		text = fmt.Sprintf("[%s] Issue milestoned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_DEMILESTONED:
		text = fmt.Sprintf("[%s] Issue demilestoned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	}

	return &SlackPayload{
		Channel:  slack.Channel,
		Text:     text,
		Username: slack.Username,
		IconURL:  slack.IconURL,
		Attachments: []*SlackAttachment{{
			Color: slack.Color,
			Title: title,
			Text:  attachmentText,
		}},
	}, nil
}

func getSlackIssueCommentPayload(p *api.IssueCommentPayload, slack *SlackMeta) (*SlackPayload, error) {
	senderLink := SlackLinkFormatter(setting.AppURL+p.Sender.UserName, p.Sender.UserName)
	titleLink := SlackLinkFormatter(fmt.Sprintf("%s/issues/%d#%s", p.Repository.HTMLURL, p.Issue.Index, CommentHashTag(p.Comment.ID)),
		fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title))
	var text, title, attachmentText string
	switch p.Action {
	case api.HOOK_ISSUE_COMMENT_CREATED:
		text = fmt.Sprintf("[%s] New comment created by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.Comment.Body)
	case api.HOOK_ISSUE_COMMENT_EDITED:
		text = fmt.Sprintf("[%s] Comment edited by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.Comment.Body)
	case api.HOOK_ISSUE_COMMENT_DELETED:
		text = fmt.Sprintf("[%s] Comment deleted by %s", p.Repository.FullName, senderLink)
		title = SlackLinkFormatter(fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index),
			fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title))
		attachmentText = SlackTextFormatter(p.Comment.Body)
	}

	return &SlackPayload{
		Channel:  slack.Channel,
		Text:     text,
		Username: slack.Username,
		IconURL:  slack.IconURL,
		Attachments: []*SlackAttachment{{
			Color: slack.Color,
			Title: title,
			Text:  attachmentText,
		}},
	}, nil
}

func getSlackPullRequestPayload(p *api.PullRequestPayload, slack *SlackMeta) (*SlackPayload, error) {
	senderLink := SlackLinkFormatter(setting.AppURL+p.Sender.UserName, p.Sender.UserName)
	titleLink := SlackLinkFormatter(fmt.Sprintf("%s/pulls/%d", p.Repository.HTMLURL, p.Index),
		fmt.Sprintf("#%d %s", p.Index, p.PullRequest.Title))
	var text, title, attachmentText string
	switch p.Action {
	case api.HOOK_ISSUE_OPENED:
		text = fmt.Sprintf("[%s] Pull request submitted by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.PullRequest.Body)
	case api.HOOK_ISSUE_CLOSED:
		if p.PullRequest.HasMerged {
			text = fmt.Sprintf("[%s] Pull request merged: %s by %s", p.Repository.FullName, titleLink, senderLink)
		} else {
			text = fmt.Sprintf("[%s] Pull request closed: %s by %s", p.Repository.FullName, titleLink, senderLink)
		}
	case api.HOOK_ISSUE_REOPENED:
		text = fmt.Sprintf("[%s] Pull request re-opened: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_EDITED:
		text = fmt.Sprintf("[%s] Pull request edited: %s by %s", p.Repository.FullName, titleLink, senderLink)
		attachmentText = SlackTextFormatter(p.PullRequest.Body)
	case api.HOOK_ISSUE_ASSIGNED:
		text = fmt.Sprintf("[%s] Pull request assigned to %s: %s by %s", p.Repository.FullName,
			SlackLinkFormatter(setting.AppURL+p.PullRequest.Assignee.UserName, p.PullRequest.Assignee.UserName),
			titleLink, senderLink)
	case api.HOOK_ISSUE_UNASSIGNED:
		text = fmt.Sprintf("[%s] Pull request unassigned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_LABEL_UPDATED:
		text = fmt.Sprintf("[%s] Pull request labels updated: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_LABEL_CLEARED:
		text = fmt.Sprintf("[%s] Pull request labels cleared: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_SYNCHRONIZED:
		text = fmt.Sprintf("[%s] Pull request synchronized: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_MILESTONED:
		text = fmt.Sprintf("[%s] Pull request milestoned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case api.HOOK_ISSUE_DEMILESTONED:
		text = fmt.Sprintf("[%s] Pull request demilestoned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	}

	return &SlackPayload{
		Channel:  slack.Channel,
		Text:     text,
		Username: slack.Username,
		IconURL:  slack.IconURL,
		Attachments: []*SlackAttachment{{
			Color: slack.Color,
			Title: title,
			Text:  attachmentText,
		}},
	}, nil
}

func getSlackReleasePayload(p *api.ReleasePayload) (*SlackPayload, error) {
	repoLink := SlackLinkFormatter(p.Repository.HTMLURL, p.Repository.Name)
	refLink := SlackLinkFormatter(p.Repository.HTMLURL+"/src/"+p.Release.TagName, p.Release.TagName)
	text := fmt.Sprintf("[%s] new release %s published by %s", repoLink, refLink, p.Sender.UserName)
	return &SlackPayload{
		Text: text,
	}, nil
}

func GetSlackPayload(p api.Payloader, event HookEventType, meta string) (payload *SlackPayload, err error) {
	slack := &SlackMeta{}
	if err := json.Unmarshal([]byte(meta), &slack); err != nil {
		return nil, fmt.Errorf("json.Unmarshal: %v", err)
	}

	switch event {
	case HOOK_EVENT_CREATE:
		payload, err = getSlackCreatePayload(p.(*api.CreatePayload))
	case HOOK_EVENT_DELETE:
		payload, err = getSlackDeletePayload(p.(*api.DeletePayload))
	case HOOK_EVENT_FORK:
		payload, err = getSlackForkPayload(p.(*api.ForkPayload))
	case HOOK_EVENT_PUSH:
		payload, err = getSlackPushPayload(p.(*api.PushPayload), slack)
	case HOOK_EVENT_ISSUES:
		payload, err = getSlackIssuesPayload(p.(*api.IssuesPayload), slack)
	case HOOK_EVENT_ISSUE_COMMENT:
		payload, err = getSlackIssueCommentPayload(p.(*api.IssueCommentPayload), slack)
	case HOOK_EVENT_PULL_REQUEST:
		payload, err = getSlackPullRequestPayload(p.(*api.PullRequestPayload), slack)
	case HOOK_EVENT_RELEASE:
		payload, err = getSlackReleasePayload(p.(*api.ReleasePayload))
	}
	if err != nil {
		return nil, fmt.Errorf("event '%s': %v", event, err)
	}

	payload.Channel = slack.Channel
	payload.Username = slack.Username
	payload.IconURL = slack.IconURL
	if len(payload.Attachments) > 0 {
		payload.Attachments[0].Color = slack.Color
	}

	return payload, nil
}
