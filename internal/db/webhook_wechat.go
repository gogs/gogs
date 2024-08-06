// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"
)

type Text struct {
	Content string `json:"content"`
}

type Markdown struct {
	Content string `json:"content"`
}

type WeChatPayload struct {
	MsgType  string   `json:"msgtype"`
	Text     Text     `json:"text"`
	Markdown Markdown `json:"markdown"`
}

func (p *WeChatPayload) JSONPayload() ([]byte, error) {
	data, err := jsoniter.MarshalIndent(p, "", "  ")
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

func NewWeChatPayload(msgType string) WeChatPayload {
	return WeChatPayload{
		MsgType: msgType,
	}
}

// getWeChatCreatePayload composes WeChat payload for create new branch or tag.
func getWeChatCreatePayload(p *api.CreatePayload) *WeChatPayload {
	refName := git.RefShortName(p.Ref)
	refType := strings.Title(p.RefType)

	markDown := NewWeChatPayload("markdown")

	markDown.Markdown.Content += "# New " + refType + " Create Event"
	markDown.Markdown.Content += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	markDown.Markdown.Content += "\n- New " + refType + ": **" + MarkdownLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName) + "**"

	return &markDown
}

// getWeChatDeletePayload composes WeChat payload for delete a branch or tag.
func getWeChatDeletePayload(p *api.DeletePayload) *WeChatPayload {
	refName := git.RefShortName(p.Ref)
	refType := strings.Title(p.RefType)

	markDown := NewWeChatPayload("markdown")

	markDown.Markdown.Content += "# " + refType + " Delete Event"
	markDown.Markdown.Content += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	markDown.Markdown.Content += "\n- " + refType + ": **" + refName + "**"

	return &markDown
}

// getWeChatForkPayload composes WeChat payload for forked by a repository.
func getWeChatForkPayload(p *api.ForkPayload) *WeChatPayload {
	markDown := NewWeChatPayload("markdown")

	markDown.Markdown.Content += "# Repo Fork Event"
	markDown.Markdown.Content += "\n- From Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	markDown.Markdown.Content += "\n- To Repo: **" + MarkdownLinkFormatter(p.Forkee.HTMLURL, p.Forkee.FullName) + "**"

	return &markDown
}

func getWeChatPushPayload(p *api.PushPayload) *WeChatPayload {
	refName := git.RefShortName(p.Ref)

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

	markDown := NewWeChatPayload("markdown")

	markDown.Markdown.Content += "# Repo Push Event"
	markDown.Markdown.Content += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.FullName) + "**"
	markDown.Markdown.Content += "\n- Ref: **" + MarkdownLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName) + "**"
	markDown.Markdown.Content += "\n- Pusher: **" + pusher + "**"
	markDown.Markdown.Content += "\n## " + fmt.Sprintf("Total %d commits(s)", len(p.Commits))
	markDown.Markdown.Content += "\n" + detail

	return &markDown
}

func getWeChatIssuesPayload(p *api.IssuesPayload) *WeChatPayload {

	issueName := fmt.Sprintf("#%d %s", p.Index, p.Issue.Title)
	issueURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index)

	markDown := NewWeChatPayload("markdown")

	markDown.Markdown.Content += "# Repository: **" + MarkdownLinkFormatter(p.Repository.HTMLURL, p.Repository.FullName) + "\n"
	markDown.Markdown.Content += "# Issue Event: " + strings.Title(string(p.Action)) + "\n"
	markDown.Markdown.Content += "- Issue:: **" + MarkdownLinkFormatter(issueURL, issueName) + "**\n"

	if p.Action == api.HOOK_ISSUE_ASSIGNED {
		markDown.Markdown.Content += "\n- New Assignee: **" + p.Issue.Assignee.UserName + "**"
	} else if p.Action == api.HOOK_ISSUE_MILESTONED {
		markDown.Markdown.Content += "\n- New Milestone: **" + p.Issue.Milestone.Title + "**"
	} else if p.Action == api.HOOK_ISSUE_LABEL_UPDATED {
		if len(p.Issue.Labels) > 0 {
			labels := make([]string, len(p.Issue.Labels))
			for i, label := range p.Issue.Labels {
				labels[i] = "**" + label.Name + "**"
			}
			markDown.Markdown.Content += "\n- Labels: " + strings.Join(labels, ",")
		} else {
			markDown.Markdown.Content += "\n- Labels: **empty**"
		}
	}

	if p.Issue.Body != "" {
		markDown.Markdown.Content += "\n> " + p.Issue.Body
	}
	return &markDown
}

func getWeChatIssueCommentPayload(p *api.IssueCommentPayload) *WeChatPayload {
	issueName := fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title)
	commentURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index)
	if p.Action != api.HOOK_ISSUE_COMMENT_DELETED {
		commentURL += "#" + CommentHashTag(p.Comment.ID)
	}

	issueURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index)

	markDown := NewWeChatPayload("markdown")

	markDown.Markdown.Content += "# Repository: **" + MarkdownLinkFormatter(p.Repository.HTMLURL, p.Repository.FullName) + "**\n"

	markDown.Markdown.Content += "# Issue Comment" + strings.Title(string(p.Action))
	markDown.Markdown.Content += "\n- Issue: " + MarkdownLinkFormatter(issueURL, issueName)
	markDown.Markdown.Content += "\n- Comment content: "
	markDown.Markdown.Content += "\n> " + p.Comment.Body

	return &markDown
}

func getWeChatPullRequestPayload(p *api.PullRequestPayload) *WeChatPayload {
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

	markDown := NewWeChatPayload("markdown")
	markDown.Markdown.Content += "# Repository: **" + MarkdownLinkFormatter(p.Repository.HTMLURL, p.Repository.FullName) + "**\n"
	markDown.Markdown.Content += title + "\n" + content

	if p.Action == api.HOOK_ISSUE_OPENED || p.Action == api.HOOK_ISSUE_EDITED {
		markDown.Markdown.Content += "\n> " + p.PullRequest.Body
	}

	return &markDown
}

func getWeChatReleasePayload(p *api.ReleasePayload) *WeChatPayload {
	releaseURL := p.Repository.HTMLURL + "/src/" + p.Release.TagName

	author := p.Release.Author.FullName

	markDown := NewWeChatPayload("markdown")

	if author == "" {
		author = p.Release.Author.UserName
	}

	markDown.Markdown.Content += "# Repository: **" + MarkdownLinkFormatter(p.Repository.HTMLURL, p.Repository.FullName) + "**\n"
	markDown.Markdown.Content += "# New Release Published"
	markDown.Markdown.Content += "\n- Repo: " + MarkdownLinkFormatter(p.Repository.HTMLURL, p.Repository.Name)
	markDown.Markdown.Content += "\n- Tag: " + MarkdownLinkFormatter(releaseURL, p.Release.TagName)
	markDown.Markdown.Content += "\n- Author: " + author
	markDown.Markdown.Content += fmt.Sprintf("\n- Draft?: %t", p.Release.Draft)
	markDown.Markdown.Content += fmt.Sprintf("\n- Pre Release?: %t", p.Release.Prerelease)
	markDown.Markdown.Content += "\n- Title: " + p.Release.Name

	if p.Release.Body != "" {
		markDown.Markdown.Content += "\n- Note: " + p.Release.Body
	}

	return &markDown
}

func GetWeChatPayload(p api.Payloader, event HookEventType) (payload *WeChatPayload, err error) {

	switch event {
	case HOOK_EVENT_CREATE:
		payload = getWeChatCreatePayload(p.(*api.CreatePayload))
	case HOOK_EVENT_DELETE:
		payload = getWeChatDeletePayload(p.(*api.DeletePayload))
	case HOOK_EVENT_FORK:
		payload = getWeChatForkPayload(p.(*api.ForkPayload))
	case HOOK_EVENT_PUSH:
		payload = getWeChatPushPayload(p.(*api.PushPayload))
	case HOOK_EVENT_ISSUES:
		payload = getWeChatIssuesPayload(p.(*api.IssuesPayload))
	case HOOK_EVENT_ISSUE_COMMENT:
		payload = getWeChatIssueCommentPayload(p.(*api.IssueCommentPayload))
	case HOOK_EVENT_PULL_REQUEST:
		payload = getWeChatPullRequestPayload(p.(*api.PullRequestPayload))
	case HOOK_EVENT_RELEASE:
		payload = getWeChatReleasePayload(p.(*api.ReleasePayload))
	default:
		return nil, errors.Errorf("unexpected event %q", event)
	}
	return payload, nil
}
