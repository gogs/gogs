package types

import "time"

type IssueStateType string

const (
	IssueStateOpen   IssueStateType = "open"
	IssueStateClosed IssueStateType = "closed"
)

type PullRequestMeta struct {
	HasMerged bool       `json:"merged"`
	Merged    *time.Time `json:"merged_at"`
}

type Issue struct {
	ID          int64            `json:"id"`
	Index       int64            `json:"number"`
	Poster      *User            `json:"user"`
	Title       string           `json:"title"`
	Body        string           `json:"body"`
	Labels      []*IssueLabel    `json:"labels"`
	Milestone   *IssueMilestone  `json:"milestone"`
	Assignee    *User            `json:"assignee"`
	State       IssueStateType   `json:"state"`
	Comments    int              `json:"comments"`
	Created     time.Time        `json:"created_at"`
	Updated     time.Time        `json:"updated_at"`
	PullRequest *PullRequestMeta `json:"pull_request"`
}

type IssueLabel struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	URL   string `json:"url"`
}

type IssueMilestone struct {
	ID           int64          `json:"id"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	State        IssueStateType `json:"state"`
	OpenIssues   int            `json:"open_issues"`
	ClosedIssues int            `json:"closed_issues"`
	Closed       *time.Time     `json:"closed_at"`
	Deadline     *time.Time     `json:"due_on"`
}

type IssueComment struct {
	ID      int64     `json:"id"`
	HTMLURL string    `json:"html_url"`
	Poster  *User     `json:"user"`
	Body    string    `json:"body"`
	Created time.Time `json:"created_at"`
	Updated time.Time `json:"updated_at"`
}
