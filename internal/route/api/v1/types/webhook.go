package types

import (
	"encoding/json"
	"time"
)

// WebhookPayloader is implemented by webhook payload types.
type WebhookPayloader interface {
	JSONPayload() ([]byte, error)
}

func jsonPayload(p any) ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}

type WebhookPayloadUser struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	UserName string `json:"username"`
}

type WebhookPayloadCommit struct {
	ID        string              `json:"id"`
	Message   string              `json:"message"`
	URL       string              `json:"url"`
	Author    *WebhookPayloadUser `json:"author"`
	Committer *WebhookPayloadUser `json:"committer"`
	Added     []string            `json:"added"`
	Removed   []string            `json:"removed"`
	Modified  []string            `json:"modified"`
	Timestamp time.Time           `json:"timestamp"`
}

type WebhookPusherType string

const WebhookPusherTypeUser WebhookPusherType = "user"

type WebhookIssueAction string

const (
	WebhookIssueOpened       WebhookIssueAction = "opened"
	WebhookIssueClosed       WebhookIssueAction = "closed"
	WebhookIssueReopened     WebhookIssueAction = "reopened"
	WebhookIssueEdited       WebhookIssueAction = "edited"
	WebhookIssueAssigned     WebhookIssueAction = "assigned"
	WebhookIssueUnassigned   WebhookIssueAction = "unassigned"
	WebhookIssueLabelUpdated WebhookIssueAction = "label_updated"
	WebhookIssueLabelCleared WebhookIssueAction = "label_cleared"
	WebhookIssueMilestoned   WebhookIssueAction = "milestoned"
	WebhookIssueDemilestoned WebhookIssueAction = "demilestoned"
	WebhookIssueSynchronized WebhookIssueAction = "synchronized"
)

type WebhookIssueCommentAction string

const (
	WebhookIssueCommentCreated WebhookIssueCommentAction = "created"
	WebhookIssueCommentEdited  WebhookIssueCommentAction = "edited"
	WebhookIssueCommentDeleted WebhookIssueCommentAction = "deleted"
)

type WebhookReleaseAction string

const WebhookReleasePublished WebhookReleaseAction = "published"

type WebhookChangesFromPayload struct {
	From string `json:"from"`
}

type WebhookChangesPayload struct {
	Title *WebhookChangesFromPayload `json:"title,omitempty"`
	Body  *WebhookChangesFromPayload `json:"body,omitempty"`
}

type WebhookCreatePayload struct {
	Ref           string      `json:"ref"`
	RefType       string      `json:"ref_type"`
	Sha           string      `json:"sha"`
	DefaultBranch string      `json:"default_branch"`
	Repo          *Repository `json:"repository"`
	Sender        *User       `json:"sender"`
}

func (p *WebhookCreatePayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }

type WebhookDeletePayload struct {
	Ref        string            `json:"ref"`
	RefType    string            `json:"ref_type"`
	PusherType WebhookPusherType `json:"pusher_type"`
	Repo       *Repository       `json:"repository"`
	Sender     *User             `json:"sender"`
}

func (p *WebhookDeletePayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }

type WebhookForkPayload struct {
	Forkee *Repository `json:"forkee"`
	Repo   *Repository `json:"repository"`
	Sender *User       `json:"sender"`
}

func (p *WebhookForkPayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }

type WebhookPushPayload struct {
	Ref        string                  `json:"ref"`
	Before     string                  `json:"before"`
	After      string                  `json:"after"`
	CompareURL string                  `json:"compare_url"`
	Commits    []*WebhookPayloadCommit `json:"commits"`
	Repo       *Repository             `json:"repository"`
	Pusher     *User                   `json:"pusher"`
	Sender     *User                   `json:"sender"`
}

func (p *WebhookPushPayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }

type WebhookIssuesPayload struct {
	Action     WebhookIssueAction     `json:"action"`
	Index      int64                  `json:"number"`
	Issue      *Issue                 `json:"issue"`
	Changes    *WebhookChangesPayload `json:"changes,omitempty"`
	Repository *Repository            `json:"repository"`
	Sender     *User                  `json:"sender"`
}

func (p *WebhookIssuesPayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }

type WebhookIssueCommentPayload struct {
	Action     WebhookIssueCommentAction `json:"action"`
	Issue      *Issue                    `json:"issue"`
	Comment    *IssueComment             `json:"comment"`
	Changes    *WebhookChangesPayload    `json:"changes,omitempty"`
	Repository *Repository               `json:"repository"`
	Sender     *User                     `json:"sender"`
}

func (p *WebhookIssueCommentPayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }

type WebhookPullRequestPayload struct {
	Action      WebhookIssueAction     `json:"action"`
	Index       int64                  `json:"number"`
	PullRequest *PullRequest           `json:"pull_request"`
	Changes     *WebhookChangesPayload `json:"changes,omitempty"`
	Repository  *Repository            `json:"repository"`
	Sender      *User                  `json:"sender"`
}

func (p *WebhookPullRequestPayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }

type WebhookReleasePayload struct {
	Action     WebhookReleaseAction `json:"action"`
	Release    *RepositoryRelease   `json:"release"`
	Repository *Repository          `json:"repository"`
	Sender     *User                `json:"sender"`
}

func (p *WebhookReleasePayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }
