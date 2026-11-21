package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
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

// SlackTextFormatter escapes the control characters of Slack, see
// https://api.slack.com/docs/formatting.
func SlackTextFormatter(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func SlackShortTextFormatter(s string) string {
	s = strings.Split(s, "\n")[0]
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func SlackLinkFormatter(url, text string) string {
	return fmt.Sprintf("<%s|%s>", url, SlackTextFormatter(text))
}

// getSlackCreatePayload composes Slack payload for creating a new branch or tag.
func getSlackCreatePayload(p *v1types.WebhookCreatePayload) *SlackPayload {
	refName := git.RefShortName(p.Ref)
	repoLink := SlackLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	refLink := SlackLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName)
	text := fmt.Sprintf("[%s:%s] %s created by %s", repoLink, refLink, p.RefType, p.Sender.UserName)
	return &SlackPayload{
		Text: text,
	}
}

// getSlackDeletePayload composes Slack payload for deleting a branch or tag.
func getSlackDeletePayload(p *v1types.WebhookDeletePayload) *SlackPayload {
	refName := git.RefShortName(p.Ref)
	repoLink := SlackLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	text := fmt.Sprintf("[%s:%s] %s deleted by %s", repoLink, refName, p.RefType, p.Sender.UserName)
	return &SlackPayload{
		Text: text,
	}
}

// getSlackForkPayload composes Slack payload for forking a repository.
func getSlackForkPayload(p *v1types.WebhookForkPayload) *SlackPayload {
	baseLink := SlackLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	forkLink := SlackLinkFormatter(p.Forkee.HTMLURL, p.Forkee.FullName)
	text := fmt.Sprintf("%s is forked to %s", baseLink, forkLink)
	return &SlackPayload{
		Text: text,
	}
}

func getSlackPushPayload(p *v1types.WebhookPushPayload, slack *SlackMeta) *SlackPayload {
	// n new commits
	var (
		branchName   = git.RefShortName(p.Ref)
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

	var attachmentText strings.Builder
	// For each commit, generate attachment text.
	for i, commit := range p.Commits {
		fmt.Fprintf(&attachmentText, "%s: %s - %s", SlackLinkFormatter(commit.URL, commit.ID[:7]), SlackShortTextFormatter(commit.Message), SlackTextFormatter(commit.Author.Name))
		// Add linebreak to each commit but the last.
		if i < len(p.Commits)-1 {
			attachmentText.WriteString("\n")
		}
	}

	return &SlackPayload{
		Channel:  slack.Channel,
		Text:     text,
		Username: slack.Username,
		IconURL:  slack.IconURL,
		Attachments: []*SlackAttachment{{
			Color: slack.Color,
			Text:  attachmentText.String(),
		}},
	}
}

func getSlackIssuesPayload(p *v1types.WebhookIssuesPayload, slack *SlackMeta) *SlackPayload {
	senderLink := SlackLinkFormatter(conf.Server.ExternalURL+p.Sender.UserName, p.Sender.UserName)
	titleLink := SlackLinkFormatter(fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index),
		fmt.Sprintf("#%d %s", p.Index, p.Issue.Title))
	var text, title, attachmentText string
	switch p.Action {
	case v1types.WebhookIssueOpened:
		text = fmt.Sprintf("[%s] New issue created by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.Issue.Body)
	case v1types.WebhookIssueClosed:
		text = fmt.Sprintf("[%s] Issue closed: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueReopened:
		text = fmt.Sprintf("[%s] Issue re-opened: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueEdited:
		text = fmt.Sprintf("[%s] Issue edited: %s by %s", p.Repository.FullName, titleLink, senderLink)
		attachmentText = SlackTextFormatter(p.Issue.Body)
	case v1types.WebhookIssueAssigned:
		text = fmt.Sprintf("[%s] Issue assigned to %s: %s by %s", p.Repository.FullName,
			SlackLinkFormatter(conf.Server.ExternalURL+p.Issue.Assignee.UserName, p.Issue.Assignee.UserName),
			titleLink, senderLink)
	case v1types.WebhookIssueUnassigned:
		text = fmt.Sprintf("[%s] Issue unassigned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueLabelUpdated:
		text = fmt.Sprintf("[%s] Issue labels updated: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueLabelCleared:
		text = fmt.Sprintf("[%s] Issue labels cleared: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueMilestoned:
		text = fmt.Sprintf("[%s] Issue milestoned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueDemilestoned:
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
	}
}

func getSlackIssueCommentPayload(p *v1types.WebhookIssueCommentPayload, slack *SlackMeta) *SlackPayload {
	senderLink := SlackLinkFormatter(conf.Server.ExternalURL+p.Sender.UserName, p.Sender.UserName)
	titleLink := SlackLinkFormatter(fmt.Sprintf("%s/issues/%d#%s", p.Repository.HTMLURL, p.Issue.Index, CommentHashTag(p.Comment.ID)),
		fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title))
	var text, title, attachmentText string
	switch p.Action {
	case v1types.WebhookIssueCommentCreated:
		text = fmt.Sprintf("[%s] New comment created by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.Comment.Body)
	case v1types.WebhookIssueCommentEdited:
		text = fmt.Sprintf("[%s] Comment edited by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.Comment.Body)
	case v1types.WebhookIssueCommentDeleted:
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
	}
}

func getSlackPullRequestPayload(p *v1types.WebhookPullRequestPayload, slack *SlackMeta) *SlackPayload {
	senderLink := SlackLinkFormatter(conf.Server.ExternalURL+p.Sender.UserName, p.Sender.UserName)
	titleLink := SlackLinkFormatter(fmt.Sprintf("%s/pulls/%d", p.Repository.HTMLURL, p.Index),
		fmt.Sprintf("#%d %s", p.Index, p.PullRequest.Title))
	var text, title, attachmentText string
	switch p.Action {
	case v1types.WebhookIssueOpened:
		text = fmt.Sprintf("[%s] Pull request submitted by %s", p.Repository.FullName, senderLink)
		title = titleLink
		attachmentText = SlackTextFormatter(p.PullRequest.Body)
	case v1types.WebhookIssueClosed:
		if p.PullRequest.HasMerged {
			text = fmt.Sprintf("[%s] Pull request merged: %s by %s", p.Repository.FullName, titleLink, senderLink)
		} else {
			text = fmt.Sprintf("[%s] Pull request closed: %s by %s", p.Repository.FullName, titleLink, senderLink)
		}
	case v1types.WebhookIssueReopened:
		text = fmt.Sprintf("[%s] Pull request re-opened: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueEdited:
		text = fmt.Sprintf("[%s] Pull request edited: %s by %s", p.Repository.FullName, titleLink, senderLink)
		attachmentText = SlackTextFormatter(p.PullRequest.Body)
	case v1types.WebhookIssueAssigned:
		text = fmt.Sprintf("[%s] Pull request assigned to %s: %s by %s", p.Repository.FullName,
			SlackLinkFormatter(conf.Server.ExternalURL+p.PullRequest.Assignee.UserName, p.PullRequest.Assignee.UserName),
			titleLink, senderLink)
	case v1types.WebhookIssueUnassigned:
		text = fmt.Sprintf("[%s] Pull request unassigned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueLabelUpdated:
		text = fmt.Sprintf("[%s] Pull request labels updated: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueLabelCleared:
		text = fmt.Sprintf("[%s] Pull request labels cleared: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueSynchronized:
		text = fmt.Sprintf("[%s] Pull request synchronized: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueMilestoned:
		text = fmt.Sprintf("[%s] Pull request milestoned: %s by %s", p.Repository.FullName, titleLink, senderLink)
	case v1types.WebhookIssueDemilestoned:
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
	}
}

func getSlackReleasePayload(p *v1types.WebhookReleasePayload) *SlackPayload {
	repoLink := SlackLinkFormatter(p.Repository.HTMLURL, p.Repository.Name)
	refLink := SlackLinkFormatter(p.Repository.HTMLURL+"/src/"+p.Release.TagName, p.Release.TagName)
	text := fmt.Sprintf("[%s] new release %s published by %s", repoLink, refLink, p.Sender.UserName)
	return &SlackPayload{
		Text: text,
	}
}

func GetSlackPayload(p v1types.WebhookPayloader, event HookEventType, meta string) (payload *SlackPayload, err error) {
	slack := &SlackMeta{}
	if err := json.Unmarshal([]byte(meta), slack); err != nil {
		return nil, errors.Newf("unmarshal: %v", err)
	}

	switch event {
	case HookEventTypeCreate:
		payload = getSlackCreatePayload(p.(*v1types.WebhookCreatePayload))
	case HookEventTypeDelete:
		payload = getSlackDeletePayload(p.(*v1types.WebhookDeletePayload))
	case HookEventTypeFork:
		payload = getSlackForkPayload(p.(*v1types.WebhookForkPayload))
	case HookEventTypePush:
		payload = getSlackPushPayload(p.(*v1types.WebhookPushPayload), slack)
	case HookEventTypeIssues:
		payload = getSlackIssuesPayload(p.(*v1types.WebhookIssuesPayload), slack)
	case HookEventTypeIssueComment:
		payload = getSlackIssueCommentPayload(p.(*v1types.WebhookIssueCommentPayload), slack)
	case HookEventTypePullRequest:
		payload = getSlackPullRequestPayload(p.(*v1types.WebhookPullRequestPayload), slack)
	case HookEventTypeRelease:
		payload = getSlackReleasePayload(p.(*v1types.WebhookReleasePayload))
	default:
		return nil, errors.Errorf("unexpected event %q", event)
	}

	payload.Channel = slack.Channel
	payload.Username = slack.Username
	payload.IconURL = slack.IconURL
	if len(payload.Attachments) > 0 {
		payload.Attachments[0].Color = slack.Color
	}
	return payload, nil
}
