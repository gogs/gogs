package types

import "encoding/json"

// Payloader is implemented by webhook payload types.
type Payloader interface {
	JSONPayload() ([]byte, error)
}

func jsonPayload(p any) ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}

type PusherType string

const PusherTypeUser PusherType = "user"

type HookIssueAction string

const (
	HookIssueOpened       HookIssueAction = "opened"
	HookIssueClosed       HookIssueAction = "closed"
	HookIssueReopened     HookIssueAction = "reopened"
	HookIssueEdited       HookIssueAction = "edited"
	HookIssueAssigned     HookIssueAction = "assigned"
	HookIssueUnassigned   HookIssueAction = "unassigned"
	HookIssueLabelUpdated HookIssueAction = "label_updated"
	HookIssueLabelCleared HookIssueAction = "label_cleared"
	HookIssueMilestoned   HookIssueAction = "milestoned"
	HookIssueDemilestoned HookIssueAction = "demilestoned"
	HookIssueSynchronized HookIssueAction = "synchronized"
)

type HookIssueCommentAction string

const (
	HookIssueCommentCreated HookIssueCommentAction = "created"
	HookIssueCommentEdited  HookIssueCommentAction = "edited"
	HookIssueCommentDeleted HookIssueCommentAction = "deleted"
)

type HookReleaseAction string

const HookReleasePublished HookReleaseAction = "published"

type ChangesFromPayload struct {
	From string `json:"from"`
}

type ChangesPayload struct {
	Title *ChangesFromPayload `json:"title,omitempty"`
	Body  *ChangesFromPayload `json:"body,omitempty"`
}

func (p *CreatePayload) JSONPayload() ([]byte, error)       { return jsonPayload(p) }
func (p *DeletePayload) JSONPayload() ([]byte, error)       { return jsonPayload(p) }
func (p *ForkPayload) JSONPayload() ([]byte, error)         { return jsonPayload(p) }
func (p *PushPayload) JSONPayload() ([]byte, error)         { return jsonPayload(p) }
func (p *IssuesPayload) JSONPayload() ([]byte, error)       { return jsonPayload(p) }
func (p *IssueCommentPayload) JSONPayload() ([]byte, error) { return jsonPayload(p) }
func (p *PullRequestPayload) JSONPayload() ([]byte, error)  { return jsonPayload(p) }
func (p *ReleasePayload) JSONPayload() ([]byte, error)      { return jsonPayload(p) }

type CreatePayload struct {
	Ref           string      `json:"ref"`
	RefType       string      `json:"ref_type"`
	Sha           string      `json:"sha"`
	DefaultBranch string      `json:"default_branch"`
	Repo          *Repository `json:"repository"`
	Sender        *User       `json:"sender"`
}

type DeletePayload struct {
	Ref        string      `json:"ref"`
	RefType    string      `json:"ref_type"`
	PusherType PusherType  `json:"pusher_type"`
	Repo       *Repository `json:"repository"`
	Sender     *User       `json:"sender"`
}

type ForkPayload struct {
	Forkee *Repository `json:"forkee"`
	Repo   *Repository `json:"repository"`
	Sender *User       `json:"sender"`
}

type PushPayload struct {
	Ref        string           `json:"ref"`
	Before     string           `json:"before"`
	After      string           `json:"after"`
	CompareURL string           `json:"compare_url"`
	Commits    []*PayloadCommit `json:"commits"`
	Repo       *Repository      `json:"repository"`
	Pusher     *User            `json:"pusher"`
	Sender     *User            `json:"sender"`
}

type IssuesPayload struct {
	Action     HookIssueAction `json:"action"`
	Index      int64           `json:"number"`
	Issue      *Issue          `json:"issue"`
	Changes    *ChangesPayload `json:"changes,omitempty"`
	Repository *Repository     `json:"repository"`
	Sender     *User           `json:"sender"`
}

type IssueCommentPayload struct {
	Action     HookIssueCommentAction `json:"action"`
	Issue      *Issue                 `json:"issue"`
	Comment    *Comment               `json:"comment"`
	Changes    *ChangesPayload        `json:"changes,omitempty"`
	Repository *Repository            `json:"repository"`
	Sender     *User                  `json:"sender"`
}

type PullRequestPayload struct {
	Action      HookIssueAction `json:"action"`
	Index       int64           `json:"number"`
	PullRequest *PullRequest    `json:"pull_request"`
	Changes     *ChangesPayload `json:"changes,omitempty"`
	Repository  *Repository     `json:"repository"`
	Sender      *User           `json:"sender"`
}

type ReleasePayload struct {
	Action     HookReleaseAction `json:"action"`
	Release    *Release          `json:"release"`
	Repository *Repository       `json:"repository"`
	Sender     *User             `json:"sender"`
}
