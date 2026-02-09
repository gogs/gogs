package database

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
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

func DiscordLinkFormatter(url, text string) string {
	return fmt.Sprintf("[%s](%s)", text, url)
}

func DiscordSHALinkFormatter(url, text string) string {
	return fmt.Sprintf("[`%s`](%s)", text, url)
}

// getDiscordCreatePayload composes Discord payload for create new branch or tag.
func getDiscordCreatePayload(p *apiv1types.WebhookCreatePayload) *DiscordPayload {
	refName := git.RefShortName(p.Ref)
	repoLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	refLink := DiscordLinkFormatter(p.Repo.HTMLURL+"/src/"+refName, refName)
	content := fmt.Sprintf("Created new %s: %s/%s", p.RefType, repoLink, refLink)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         conf.Server.ExternalURL + p.Sender.UserName,
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarURL,
			},
		}},
	}
}

// getDiscordDeletePayload composes Discord payload for delete a branch or tag.
func getDiscordDeletePayload(p *apiv1types.WebhookDeletePayload) *DiscordPayload {
	refName := git.RefShortName(p.Ref)
	repoLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	content := fmt.Sprintf("Deleted %s: %s/%s", p.RefType, repoLink, refName)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         conf.Server.ExternalURL + p.Sender.UserName,
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarURL,
			},
		}},
	}
}

// getDiscordForkPayload composes Discord payload for forked by a repository.
func getDiscordForkPayload(p *apiv1types.WebhookForkPayload) *DiscordPayload {
	baseLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	forkLink := DiscordLinkFormatter(p.Forkee.HTMLURL, p.Forkee.FullName)
	content := fmt.Sprintf("%s is forked to %s", baseLink, forkLink)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         conf.Server.ExternalURL + p.Sender.UserName,
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarURL,
			},
		}},
	}
}

func getDiscordPushPayload(p *apiv1types.WebhookPushPayload, slack *SlackMeta) *DiscordPayload {
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
		commitString = DiscordLinkFormatter(p.CompareURL, commitDesc)
	} else {
		commitString = commitDesc
	}

	repoLink := DiscordLinkFormatter(p.Repo.HTMLURL, p.Repo.Name)
	branchLink := DiscordLinkFormatter(p.Repo.HTMLURL+"/src/"+branchName, branchName)
	var content strings.Builder
	content.WriteString(fmt.Sprintf("Pushed %s to %s/%s\n", commitString, repoLink, branchLink))

	// for each commit, generate attachment text
	for i, commit := range p.Commits {
		content.WriteString(fmt.Sprintf("%s %s - %s", DiscordSHALinkFormatter(commit.URL, commit.ID[:7]), DiscordTextFormatter(commit.Message), commit.Author.Name))
		// add linebreak to each commit but the last
		if i < len(p.Commits)-1 {
			content.WriteString("\n")
		}
	}

	color, _ := strconv.ParseInt(strings.TrimLeft(slack.Color, "#"), 16, 32)
	return &DiscordPayload{
		Username:  slack.Username,
		AvatarURL: slack.IconURL,
		Embeds: []*DiscordEmbedObject{{
			Description: content.String(),
			URL:         conf.Server.ExternalURL + p.Sender.UserName,
			Color:       int(color),
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarURL,
			},
		}},
	}
}

func getDiscordIssuesPayload(p *apiv1types.WebhookIssuesPayload, slack *SlackMeta) *DiscordPayload {
	title := fmt.Sprintf("#%d %s", p.Index, p.Issue.Title)
	url := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index)
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case apiv1types.WebhookIssueOpened:
		title = "New issue: " + title
		content = p.Issue.Body
	case apiv1types.WebhookIssueClosed:
		title = "Issue closed: " + title
	case apiv1types.WebhookIssueReopened:
		title = "Issue re-opened: " + title
	case apiv1types.WebhookIssueEdited:
		title = "Issue edited: " + title
		content = p.Issue.Body
	case apiv1types.WebhookIssueAssigned:
		title = "Issue assigned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Assignee",
			Value: p.Issue.Assignee.UserName,
		}}
	case apiv1types.WebhookIssueUnassigned:
		title = "Issue unassigned: " + title
	case apiv1types.WebhookIssueLabelUpdated:
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
	case apiv1types.WebhookIssueLabelCleared:
		title = "Issue labels cleared: " + title
	case apiv1types.WebhookIssueSynchronized:
		title = "Issue synchronized: " + title
	case apiv1types.WebhookIssueMilestoned:
		title = "Issue milestoned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Milestone",
			Value: p.Issue.Milestone.Title,
		}}
	case apiv1types.WebhookIssueDemilestoned:
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
				IconURL: p.Sender.AvatarURL,
			},
			Fields: fields,
		}},
	}
}

func getDiscordIssueCommentPayload(p *apiv1types.WebhookIssueCommentPayload, slack *SlackMeta) *DiscordPayload {
	title := fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title)
	url := fmt.Sprintf("%s/issues/%d#%s", p.Repository.HTMLURL, p.Issue.Index, CommentHashTag(p.Comment.ID))
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case apiv1types.WebhookIssueCommentCreated:
		title = "New comment: " + title
		content = p.Comment.Body
	case apiv1types.WebhookIssueCommentEdited:
		title = "Comment edited: " + title
		content = p.Comment.Body
	case apiv1types.WebhookIssueCommentDeleted:
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
				IconURL: p.Sender.AvatarURL,
			},
			Fields: fields,
		}},
	}
}

func getDiscordPullRequestPayload(p *apiv1types.WebhookPullRequestPayload, slack *SlackMeta) *DiscordPayload {
	title := fmt.Sprintf("#%d %s", p.Index, p.PullRequest.Title)
	url := fmt.Sprintf("%s/pulls/%d", p.Repository.HTMLURL, p.Index)
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case apiv1types.WebhookIssueOpened:
		title = "New pull request: " + title
		content = p.PullRequest.Body
	case apiv1types.WebhookIssueClosed:
		if p.PullRequest.HasMerged {
			title = "Pull request merged: " + title
		} else {
			title = "Pull request closed: " + title
		}
	case apiv1types.WebhookIssueReopened:
		title = "Pull request re-opened: " + title
	case apiv1types.WebhookIssueEdited:
		title = "Pull request edited: " + title
		content = p.PullRequest.Body
	case apiv1types.WebhookIssueAssigned:
		title = "Pull request assigned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Assignee",
			Value: p.PullRequest.Assignee.UserName,
		}}
	case apiv1types.WebhookIssueUnassigned:
		title = "Pull request unassigned: " + title
	case apiv1types.WebhookIssueLabelUpdated:
		title = "Pull request labels updated: " + title
		labels := make([]string, len(p.PullRequest.Labels))
		for i := range p.PullRequest.Labels {
			labels[i] = p.PullRequest.Labels[i].Name
		}
		fields = []*DiscordEmbedFieldObject{{
			Name:  "Labels",
			Value: strings.Join(labels, ", "),
		}}
	case apiv1types.WebhookIssueLabelCleared:
		title = "Pull request labels cleared: " + title
	case apiv1types.WebhookIssueSynchronized:
		title = "Pull request synchronized: " + title
	case apiv1types.WebhookIssueMilestoned:
		title = "Pull request milestoned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Milestone",
			Value: p.PullRequest.Milestone.Title,
		}}
	case apiv1types.WebhookIssueDemilestoned:
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
				IconURL: p.Sender.AvatarURL,
			},
			Fields: fields,
		}},
	}
}

func getDiscordReleasePayload(p *apiv1types.WebhookReleasePayload) *DiscordPayload {
	repoLink := DiscordLinkFormatter(p.Repository.HTMLURL, p.Repository.Name)
	refLink := DiscordLinkFormatter(p.Repository.HTMLURL+"/src/"+p.Release.TagName, p.Release.TagName)
	content := fmt.Sprintf("Published new release %s of %s", refLink, repoLink)
	return &DiscordPayload{
		Embeds: []*DiscordEmbedObject{{
			Description: content,
			URL:         conf.Server.ExternalURL + p.Sender.UserName,
			Author: &DiscordEmbedAuthorObject{
				Name:    p.Sender.UserName,
				IconURL: p.Sender.AvatarURL,
			},
		}},
	}
}

func GetDiscordPayload(p apiv1types.WebhookPayloader, event HookEventType, meta string) (payload *DiscordPayload, err error) {
	slack := &SlackMeta{}
	if err := json.Unmarshal([]byte(meta), slack); err != nil {
		return nil, errors.Newf("unmarshal: %v", err)
	}

	switch event {
	case HookEventTypeCreate:
		payload = getDiscordCreatePayload(p.(*apiv1types.WebhookCreatePayload))
	case HookEventTypeDelete:
		payload = getDiscordDeletePayload(p.(*apiv1types.WebhookDeletePayload))
	case HookEventTypeFork:
		payload = getDiscordForkPayload(p.(*apiv1types.WebhookForkPayload))
	case HookEventTypePush:
		payload = getDiscordPushPayload(p.(*apiv1types.WebhookPushPayload), slack)
	case HookEventTypeIssues:
		payload = getDiscordIssuesPayload(p.(*apiv1types.WebhookIssuesPayload), slack)
	case HookEventTypeIssueComment:
		payload = getDiscordIssueCommentPayload(p.(*apiv1types.WebhookIssueCommentPayload), slack)
	case HookEventTypePullRequest:
		payload = getDiscordPullRequestPayload(p.(*apiv1types.WebhookPullRequestPayload), slack)
	case HookEventTypeRelease:
		payload = getDiscordReleasePayload(p.(*apiv1types.WebhookReleasePayload))
	default:
		return nil, errors.Errorf("unexpected event %q", event)
	}

	payload.Username = slack.Username
	payload.AvatarURL = slack.IconURL
	if len(payload.Embeds) > 0 {
		color, _ := strconv.ParseInt(strings.TrimLeft(slack.Color, "#"), 16, 32)
		payload.Embeds[0].Color = int(color)
	}
	return payload, nil
}
