package types

import "time"

type RepositoryPermission struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

type Repository struct {
	ID            int64                 `json:"id"`
	Owner         *User                 `json:"owner"`
	Name          string                `json:"name"`
	FullName      string                `json:"full_name"`
	Description   string                `json:"description"`
	Private       bool                  `json:"private"`
	Fork          bool                  `json:"fork"`
	Parent        *Repository           `json:"parent"`
	Empty         bool                  `json:"empty"`
	Mirror        bool                  `json:"mirror"`
	Size          int64                 `json:"size"`
	HTMLURL       string                `json:"html_url"`
	SSHURL        string                `json:"ssh_url"`
	CloneURL      string                `json:"clone_url"`
	Website       string                `json:"website"`
	Stars         int                   `json:"stars_count"`
	Forks         int                   `json:"forks_count"`
	Watchers      int                   `json:"watchers_count"`
	OpenIssues    int                   `json:"open_issues_count"`
	DefaultBranch string                `json:"default_branch"`
	Created       time.Time             `json:"created_at"`
	Updated       time.Time             `json:"updated_at"`
	Permissions   *RepositoryPermission `json:"permissions,omitempty"`
}

type RepositoryBranch struct {
	Name   string                `json:"name"`
	Commit *WebhookPayloadCommit `json:"commit"`
}

type RepositoryRelease struct {
	ID              int64     `json:"id"`
	TagName         string    `json:"tag_name"`
	TargetCommitish string    `json:"target_commitish"`
	Name            string    `json:"name"`
	Body            string    `json:"body"`
	Draft           bool      `json:"draft"`
	Prerelease      bool      `json:"prerelease"`
	Author          *User     `json:"author"`
	Created         time.Time `json:"created_at"`
}
