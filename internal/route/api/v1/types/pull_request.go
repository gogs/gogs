package types

import "time"

type PullRequest struct {
	ID             int64           `json:"id"`
	Index          int64           `json:"number"`
	Poster         *User           `json:"user"`
	Title          string          `json:"title"`
	Body           string          `json:"body"`
	Labels         []*IssueLabel   `json:"labels"`
	Milestone      *IssueMilestone `json:"milestone"`
	Assignee       *User           `json:"assignee"`
	State          IssueStateType  `json:"state"`
	Comments       int             `json:"comments"`
	HeadBranch     string          `json:"head_branch"`
	HeadRepo       *Repository     `json:"head_repo"`
	BaseBranch     string          `json:"base_branch"`
	BaseRepo       *Repository     `json:"base_repo"`
	HTMLURL        string          `json:"html_url"`
	Mergeable      *bool           `json:"mergeable"`
	HasMerged      bool            `json:"merged"`
	Merged         *time.Time      `json:"merged_at"`
	MergedCommitID *string         `json:"merge_commit_sha"`
	MergedBy       *User           `json:"merged_by"`
}
