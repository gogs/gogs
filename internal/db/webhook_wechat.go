// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"

	api "github.com/gogs/go-gogs-client"
)

type Text struct {
	Content string `json:"content"`
}

type WeChatPayload struct {
	MsgType string `json:"msgtype"`
	Text    Text   `json:"text"`
}

func (p *WeChatPayload) JSONPayload() ([]byte, error) {
	data, err := jsoniter.MarshalIndent(p, "", "  ")
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

// see: https://api.WeChat.com/docs/formatting
func WeChatTextFormatter(s string) string {
	// replace & < >
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func WeChatShortTextFormatter(s string) string {
	s = strings.Split(s, "\n")[0]
	// replace & < >
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func WeChatLinkFormatter(url, text string) string {
	return fmt.Sprintf("<%s|%s>", url, WeChatTextFormatter(text))
}

// getWeChatCreatePayload composes WeChat payload for create new branch or tag.
func getWeChatCreatePayload(p *api.CreatePayload) *WeChatPayload {

	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatCreatePayload",
		},
	}
}

// getWeChatDeletePayload composes WeChat payload for delete a branch or tag.
func getWeChatDeletePayload(p *api.DeletePayload) *WeChatPayload {
	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatDeletePayload",
		},
	}
}

// getWeChatForkPayload composes WeChat payload for forked by a repository.
func getWeChatForkPayload(p *api.ForkPayload) *WeChatPayload {
	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatForkPayload",
		},
	}
}

func getWeChatPushPayload(p *api.PushPayload) *WeChatPayload {
	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatPushPayload",
		},
	}
}

func getWeChatIssuesPayload(p *api.IssuesPayload) *WeChatPayload {

	issueName := fmt.Sprintf("#%d %s", p.Index, p.Issue.Title)
	issueURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index)

	fmt.Println(issueName, issueURL)

	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatIssuesPayload",
		},
	}
}

func getWeChatIssueCommentPayload(p *api.IssueCommentPayload) *WeChatPayload {
	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatIssueCommentPayload",
		},
	}
}

func getWeChatPullRequestPayload(p *api.PullRequestPayload) *WeChatPayload {
	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatPullRequestPayload",
		},
	}
}

func getWeChatReleasePayload(p *api.ReleasePayload) *WeChatPayload {
	return &WeChatPayload{
		MsgType: "text",
		Text: Text{
			Content: "getWeChatReleasePayload",
		},
	}
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
