package database

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
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
	Content string `json:"content"`
	// Discord rejects an empty username, so it is left out to use the name
	// set on the webhook in Discord.
	Username  string                `json:"username,omitempty"`
	AvatarURL string                `json:"avatar_url,omitempty"`
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

// getDiscordCreatePayload composes Discord payload for creating a new branch or tag.
func getDiscordCreatePayload(p *v1types.WebhookCreatePayload) *DiscordPayload {
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

// getDiscordDeletePayload composes Discord payload for deleting a branch or tag.
func getDiscordDeletePayload(p *v1types.WebhookDeletePayload) *DiscordPayload {
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

// getDiscordForkPayload composes Discord payload for forking a repository.
func getDiscordForkPayload(p *v1types.WebhookForkPayload) *DiscordPayload {
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

func getDiscordPushPayload(p *v1types.WebhookPushPayload, slack *SlackMeta) *DiscordPayload {
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
	fmt.Fprintf(&content, "Pushed %s to %s/%s\n", commitString, repoLink, branchLink)

	// For each commit, generate attachment text.
	for i, commit := range p.Commits {
		fmt.Fprintf(&content, "%s %s - %s", DiscordSHALinkFormatter(commit.URL, commit.ID[:7]), DiscordTextFormatter(commit.Message), commit.Author.Name)
		// Add linebreak to each commit but the last.
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

func getDiscordIssuesPayload(p *v1types.WebhookIssuesPayload, slack *SlackMeta) *DiscordPayload {
	title := fmt.Sprintf("#%d %s", p.Index, p.Issue.Title)
	url := fmt.Sprintf("%s/issues/%d", p.Repository.HTMLURL, p.Index)
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case v1types.WebhookIssueOpened:
		title = "New issue: " + title
		content = p.Issue.Body
	case v1types.WebhookIssueClosed:
		title = "Issue closed: " + title
	case v1types.WebhookIssueReopened:
		title = "Issue re-opened: " + title
	case v1types.WebhookIssueEdited:
		title = "Issue edited: " + title
		content = p.Issue.Body
	case v1types.WebhookIssueAssigned:
		title = "Issue assigned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Assignee",
			Value: p.Issue.Assignee.UserName,
		}}
	case v1types.WebhookIssueUnassigned:
		title = "Issue unassigned: " + title
	case v1types.WebhookIssueLabelUpdated:
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
	case v1types.WebhookIssueLabelCleared:
		title = "Issue labels cleared: " + title
	case v1types.WebhookIssueSynchronized:
		title = "Issue synchronized: " + title
	case v1types.WebhookIssueMilestoned:
		title = "Issue milestoned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Milestone",
			Value: p.Issue.Milestone.Title,
		}}
	case v1types.WebhookIssueDemilestoned:
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

func getDiscordIssueCommentPayload(p *v1types.WebhookIssueCommentPayload, slack *SlackMeta) *DiscordPayload {
	title := fmt.Sprintf("#%d %s", p.Issue.Index, p.Issue.Title)
	url := fmt.Sprintf("%s/issues/%d#%s", p.Repository.HTMLURL, p.Issue.Index, CommentHashTag(p.Comment.ID))
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case v1types.WebhookIssueCommentCreated:
		title = "New comment: " + title
		content = p.Comment.Body
	case v1types.WebhookIssueCommentEdited:
		title = "Comment edited: " + title
		content = p.Comment.Body
	case v1types.WebhookIssueCommentDeleted:
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

func getDiscordPullRequestPayload(p *v1types.WebhookPullRequestPayload, slack *SlackMeta) *DiscordPayload {
	title := fmt.Sprintf("#%d %s", p.Index, p.PullRequest.Title)
	url := fmt.Sprintf("%s/pulls/%d", p.Repository.HTMLURL, p.Index)
	content := ""
	fields := make([]*DiscordEmbedFieldObject, 0, 1)
	switch p.Action {
	case v1types.WebhookIssueOpened:
		title = "New pull request: " + title
		content = p.PullRequest.Body
	case v1types.WebhookIssueClosed:
		if p.PullRequest.HasMerged {
			title = "Pull request merged: " + title
		} else {
			title = "Pull request closed: " + title
		}
	case v1types.WebhookIssueReopened:
		title = "Pull request re-opened: " + title
	case v1types.WebhookIssueEdited:
		title = "Pull request edited: " + title
		content = p.PullRequest.Body
	case v1types.WebhookIssueAssigned:
		title = "Pull request assigned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Assignee",
			Value: p.PullRequest.Assignee.UserName,
		}}
	case v1types.WebhookIssueUnassigned:
		title = "Pull request unassigned: " + title
	case v1types.WebhookIssueLabelUpdated:
		title = "Pull request labels updated: " + title
		labels := make([]string, len(p.PullRequest.Labels))
		for i := range p.PullRequest.Labels {
			labels[i] = p.PullRequest.Labels[i].Name
		}
		fields = []*DiscordEmbedFieldObject{{
			Name:  "Labels",
			Value: strings.Join(labels, ", "),
		}}
	case v1types.WebhookIssueLabelCleared:
		title = "Pull request labels cleared: " + title
	case v1types.WebhookIssueSynchronized:
		title = "Pull request synchronized: " + title
	case v1types.WebhookIssueMilestoned:
		title = "Pull request milestoned: " + title
		fields = []*DiscordEmbedFieldObject{{
			Name:  "New Milestone",
			Value: p.PullRequest.Milestone.Title,
		}}
	case v1types.WebhookIssueDemilestoned:
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

func getDiscordReleasePayload(p *v1types.WebhookReleasePayload) *DiscordPayload {
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

func GetDiscordPayload(p v1types.WebhookPayloader, event HookEventType, meta string) (payload *DiscordPayload, err error) {
	// Discord webhooks created through API v1 have no settings.
	slack := &SlackMeta{}
	if meta != "" {
		if err := json.Unmarshal([]byte(meta), slack); err != nil {
			return nil, errors.Newf("unmarshal: %v", err)
		}
	}

	switch event {
	case HookEventTypeCreate:
		payload = getDiscordCreatePayload(p.(*v1types.WebhookCreatePayload))
	case HookEventTypeDelete:
		payload = getDiscordDeletePayload(p.(*v1types.WebhookDeletePayload))
	case HookEventTypeFork:
		payload = getDiscordForkPayload(p.(*v1types.WebhookForkPayload))
	case HookEventTypePush:
		payload = getDiscordPushPayload(p.(*v1types.WebhookPushPayload), slack)
	case HookEventTypeIssues:
		payload = getDiscordIssuesPayload(p.(*v1types.WebhookIssuesPayload), slack)
	case HookEventTypeIssueComment:
		payload = getDiscordIssueCommentPayload(p.(*v1types.WebhookIssueCommentPayload), slack)
	case HookEventTypePullRequest:
		payload = getDiscordPullRequestPayload(p.(*v1types.WebhookPullRequestPayload), slack)
	case HookEventTypeRelease:
		payload = getDiscordReleasePayload(p.(*v1types.WebhookReleasePayload))
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
