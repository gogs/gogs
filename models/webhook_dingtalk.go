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
	case HOOK_EVENT_ISSUES:
		payload, err = getDingtalkIssuesPayload(p.(*api.IssuesPayload))
	case HOOK_EVENT_ISSUE_COMMENT:
		payload, err = getDingtalkIssueCommentPayload(p.(*api.IssueCommentPayload))
	case HOOK_EVENT_PULL_REQUEST:
		payload, err = getDingtalkPullRequestPayload(p.(*api.PullRequestPayload))
	case HOOK_EVENT_RELEASE:
		payload, err = getDingtalkReleasePayload(p.(*api.ReleasePayload))
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

func getDingtalkIssuesPayload(p *api.IssuesPayload) (*DingtalkPayload, error) {
	issueName := fmt.Sprintf("#%d %s", p.Index, p.Issue.Title)
	issueURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index)

	actionCard := NewDingtalkActionCard("View Issue", issueURL)

	actionCard.Text += "# Issue Event " + strings.Title(string(p.Action))
	actionCard.Text += "\n- Issue: **" + MarkdownLinkFormatter(issueURL, issueName) + "**"

	if p.Action == api.HOOK_ISSUE_ASSIGNED {
		actionCard.Text += "\n- New Assignee: **" + p.Issue.Assignee.UserName + "**"
	} else if p.Action == api.HOOK_ISSUE_MILESTONED {
		actionCard.Text += "\n- New Milestone: **" + p.Issue.Milestone.Title + "**"
	} else if p.Action == api.HOOK_ISSUE_LABEL_UPDATED {
		if len(p.Issue.Labels) > 0 {
			labels := make([]string, len(p.Issue.Labels))
			for i, label := range p.Issue.Labels {
				labels[i] = "**" + label.Name + "**"
			}
			actionCard.Text += "\n- Labels: " + strings.Join(labels, ",")
		} else {
			actionCard.Text += "\n- Labels: **empty**"
		}
	}

	if p.Issue.Body != "" {
		actionCard.Text += "\n> " + p.Issue.Body
	}

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

func getDingtalkIssueCommentPayload(p *api.IssueCommentPayload) (*DingtalkPayload, error) {
	issueName := fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title)
	commentURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index)
	if p.Action != api.HOOK_ISSUE_COMMENT_DELETED {
		commentURL += "#" + CommentHashTag(p.Comment.ID)
	}

	issueURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index)

	actionCard := NewDingtalkActionCard("View Issue Comment", commentURL)

	actionCard.Text += "# Issue Comment " + strings.Title(string(p.Action))
	actionCard.Text += "\n- Issue: " + MarkdownLinkFormatter(issueURL, issueName)
	actionCard.Text += "\n- Comment content: "
	actionCard.Text += "\n> " + p.Comment.Body

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

func getDingtalkPullRequestPayload(p *api.PullRequestPayload) (*DingtalkPayload, error) {
	title := "# Pull Request " + strings.Title(string(p.Action))
	if p.Action == api.HOOK_ISSUE_CLOSED && p.PullRequest.HasMerged {
		title = "# Pull Request Merged"
	}

	pullRequestURL := fmt.Sprintf("%s/pulls/%d", p.Repository.HTMLURL, p.Index)

	content := "- PR: " + MarkdownLinkFormatter(pullRequestURL, fmt.Sprintf("#%d %s", p.Index, p.PullRequest.Title))
	if p.Action == api.HOOK_ISSUE_ASSIGNED {
		content += "\n- New Assignee: **" + p.PullRequest.Assignee.UserName + "**"
	} else if p.Action == api.HOOK_ISSUE_MILESTONED {
		content += "\n- New Milestone: *" + p.PullRequest.Milestone.Title + "*"
	} else if p.Action == api.HOOK_ISSUE_LABEL_UPDATED {
		labels := make([]string, len(p.PullRequest.Labels))
		for i, label := range p.PullRequest.Labels {
			labels[i] = "**" + label.Name + "**"
		}
		content += "\n- New Labels: " + strings.Join(labels, ",")
	}

	actionCard := NewDingtalkActionCard("View Pull Request", pullRequestURL)
	actionCard.Text += title + "\n" + content

	if p.Action == api.HOOK_ISSUE_OPENED || p.Action == api.HOOK_ISSUE_EDITED {
		actionCard.Text += "\n> " + p.PullRequest.Body
	}

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

func getDingtalkReleasePayload(p *api.ReleasePayload) (*DingtalkPayload, error) {
	releaseURL := p.Repository.HTMLURL + "/src/" + p.Release.TagName

	author := p.Release.Author.FullName
	if author == "" {
		author = p.Release.Author.UserName
	}

	actionCard := NewDingtalkActionCard("View Release", releaseURL)

	actionCard.Text += "# New Release Published"
	actionCard.Text += "\n- Repo: " + MarkdownLinkFormatter(p.Repository.HTMLURL, p.Repository.Name)
	actionCard.Text += "\n- Tag: " + MarkdownLinkFormatter(releaseURL, p.Release.TagName)
	actionCard.Text += "\n- Author: " + author
	actionCard.Text += fmt.Sprintf("\n- Draft?: %t", p.Release.Draft)
	actionCard.Text += fmt.Sprintf("\n- Pre Release?: %t", p.Release.Prerelease)
	actionCard.Text += "\n- Title: " + p.Release.Name

	if p.Release.Body != "" {
		actionCard.Text += "\n- Note: " + p.Release.Body
	}

	return &DingtalkPayload{MsgType: "actionCard", ActionCard: actionCard}, nil
}

//Format link addr and title into markdown style
func MarkdownLinkFormatter(link, text string) string {
	return "[" + text + "](" + link + ")"
}
