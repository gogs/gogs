package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gogs/git-module"

	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

const (
	DingtalkNotificationTitle = "Gogs Notification"
)

// Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkActionCard struct {
	Title          string `json:"title"`
	Text           string `json:"text"`
	HideAvatar     string `json:"hideAvatar"`
	BtnOrientation string `json:"btnOrientation"`
	SingleTitle    string `json:"singleTitle"`
	SingleURL      string `json:"singleURL"`
}

// Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
type DingtalkAtObject struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

// Refer: https://open-doc.dingtalk.com/docs/doc.htm?treeId=257&articleId=105735&docType=1
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

// TODO: add content
func GetDingtalkPayload(p apiv1types.WebhookPayloader, event HookEventType) (payload *DingtalkPayload, err error) {
	switch event {
	case HookEventTypeCreate:
		payload = getDingtalkCreatePayload(p.(*apiv1types.WebhookCreatePayload))
	case HookEventTypeDelete:
		payload = getDingtalkDeletePayload(p.(*apiv1types.WebhookDeletePayload))
	case HookEventTypeFork:
		payload = getDingtalkForkPayload(p.(*apiv1types.WebhookForkPayload))
	case HookEventTypePush:
		payload = getDingtalkPushPayload(p.(*apiv1types.WebhookPushPayload))
	case HookEventTypeIssues:
		payload = getDingtalkIssuesPayload(p.(*apiv1types.WebhookIssuesPayload))
	case HookEventTypeIssueComment:
		payload = getDingtalkIssueCommentPayload(p.(*apiv1types.WebhookIssueCommentPayload))
	case HookEventTypePullRequest:
		payload = getDingtalkPullRequestPayload(p.(*apiv1types.WebhookPullRequestPayload))
	case HookEventTypeRelease:
		payload = getDingtalkReleasePayload(p.(*apiv1types.WebhookReleasePayload))
	default:
		return nil, errors.Errorf("unexpected event %q", event)
	}
	return payload, nil
}

func getDingtalkCreatePayload(p *apiv1types.WebhookCreatePayload) *DingtalkPayload {
	refName := git.RefShortName(p.Ref)
	refType := strings.Title(p.RefType)

	actionCard := NewDingtalkActionCard("View "+refType, p.Repo.HTMLURL+"/src/"+refName)
	actionCard.Text += "# New " + refType + " Create Event"
	actionCard.Text += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- New " + refType + ": **" + MarkdownLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName) + "**"

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

func getDingtalkDeletePayload(p *apiv1types.WebhookDeletePayload) *DingtalkPayload {
	refName := git.RefShortName(p.Ref)
	refType := strings.Title(p.RefType)

	actionCard := NewDingtalkActionCard("View Repo", p.Repo.HTMLURL)
	actionCard.Text += "# " + refType + " Delete Event"
	actionCard.Text += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- " + refType + ": **" + refName + "**"

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

func getDingtalkForkPayload(p *apiv1types.WebhookForkPayload) *DingtalkPayload {
	actionCard := NewDingtalkActionCard("View Fork", p.Forkee.HTMLURL)
	actionCard.Text += "# Repo Fork Event"
	actionCard.Text += "\n- From Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- To Repo: **" + MarkdownLinkFormatter(p.Forkee.HTMLURL, p.Forkee.FullName) + "**"

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

func getDingtalkPushPayload(p *apiv1types.WebhookPushPayload) *DingtalkPayload {
	refName := git.RefShortName(p.Ref)

	pusher := p.Pusher.FullName
	if pusher == "" {
		pusher = p.Pusher.UserName
	}

	var detail strings.Builder
	for i, commit := range p.Commits {
		msg := strings.Split(commit.Message, "\n")[0]
		commitLink := MarkdownLinkFormatter(commit.URL, commit.ID[:7])
		detail.WriteString(fmt.Sprintf("> %d. %s %s - %s\n", i, commitLink, commit.Author.Name, msg))
	}

	actionCard := NewDingtalkActionCard("View Changes", p.CompareURL)
	actionCard.Text += "# Repo Push Event"
	actionCard.Text += "\n- Repo: **" + MarkdownLinkFormatter(p.Repo.HTMLURL, p.Repo.Name) + "**"
	actionCard.Text += "\n- Ref: **" + MarkdownLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName) + "**"
	actionCard.Text += "\n- Pusher: **" + pusher + "**"
	actionCard.Text += "\n## " + fmt.Sprintf("Total %d commits(s)", len(p.Commits))
	actionCard.Text += "\n" + detail.String()

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

func getDingtalkIssuesPayload(p *apiv1types.WebhookIssuesPayload) *DingtalkPayload {
	issueName := fmt.Sprintf("#%d %s", p.Index, p.Issue.Title)
	issueURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index)

	actionCard := NewDingtalkActionCard("View Issue", issueURL)
	actionCard.Text += "# Issue Event " + strings.Title(string(p.Action))
	actionCard.Text += "\n- Issue: **" + MarkdownLinkFormatter(issueURL, issueName) + "**"

	switch p.Action {
	case apiv1types.WebhookIssueAssigned:
		actionCard.Text += "\n- New Assignee: **" + p.Issue.Assignee.UserName + "**"
	case apiv1types.WebhookIssueMilestoned:
		actionCard.Text += "\n- New Milestone: **" + p.Issue.Milestone.Title + "**"
	case apiv1types.WebhookIssueLabelUpdated:
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

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

func getDingtalkIssueCommentPayload(p *apiv1types.WebhookIssueCommentPayload) *DingtalkPayload {
	issueName := fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title)
	commentURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index)
	if p.Action != apiv1types.WebhookIssueCommentDeleted {
		commentURL += "#" + CommentHashTag(p.Comment.ID)
	}

	issueURL := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Issue.Index)

	actionCard := NewDingtalkActionCard("View Issue Comment", commentURL)
	actionCard.Text += "# Issue Comment " + strings.Title(string(p.Action))
	actionCard.Text += "\n- Issue: " + MarkdownLinkFormatter(issueURL, issueName)
	actionCard.Text += "\n- Comment content: "
	actionCard.Text += "\n> " + p.Comment.Body

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

func getDingtalkPullRequestPayload(p *apiv1types.WebhookPullRequestPayload) *DingtalkPayload {
	title := "# Pull Request " + strings.Title(string(p.Action))
	if p.Action == apiv1types.WebhookIssueClosed && p.PullRequest.HasMerged {
		title = "# Pull Request Merged"
	}

	pullRequestURL := fmt.Sprintf("%s/pulls/%d", p.Repository.HTMLURL, p.Index)

	content := "- PR: " + MarkdownLinkFormatter(pullRequestURL, fmt.Sprintf("#%d %s", p.Index, p.PullRequest.Title))
	switch p.Action {
	case apiv1types.WebhookIssueAssigned:
		content += "\n- New Assignee: **" + p.PullRequest.Assignee.UserName + "**"
	case apiv1types.WebhookIssueMilestoned:
		content += "\n- New Milestone: *" + p.PullRequest.Milestone.Title + "*"
	case apiv1types.WebhookIssueLabelUpdated:
		labels := make([]string, len(p.PullRequest.Labels))
		for i, label := range p.PullRequest.Labels {
			labels[i] = "**" + label.Name + "**"
		}
		content += "\n- New Labels: " + strings.Join(labels, ",")
	}

	actionCard := NewDingtalkActionCard("View Pull Request", pullRequestURL)
	actionCard.Text += title + "\n" + content

	if p.Action == apiv1types.WebhookIssueOpened || p.Action == apiv1types.WebhookIssueEdited {
		actionCard.Text += "\n> " + p.PullRequest.Body
	}

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

func getDingtalkReleasePayload(p *apiv1types.WebhookReleasePayload) *DingtalkPayload {
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

	return &DingtalkPayload{
		MsgType:    "actionCard",
		ActionCard: actionCard,
	}
}

// MarkdownLinkFormatter formats link address and title into Markdown style.
func MarkdownLinkFormatter(link, text string) string {
	return "[" + text + "](" + link + ")"
}
