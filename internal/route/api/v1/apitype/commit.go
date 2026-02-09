package apitype

import "time"

type PayloadUser struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	UserName string `json:"username"`
}

type PayloadCommit struct {
	ID        string       `json:"id"`
	Message   string       `json:"message"`
	URL       string       `json:"url"`
	Author    *PayloadUser `json:"author"`
	Committer *PayloadUser `json:"committer"`
	Added     []string     `json:"added"`
	Removed   []string     `json:"removed"`
	Modified  []string     `json:"modified"`
	Timestamp time.Time    `json:"timestamp"`
}

type CommitMeta struct {
	URL string `json:"url"`
	SHA string `json:"sha"`
}

type CommitUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

type RepoCommit struct {
	URL       string      `json:"url"`
	Author    *CommitUser `json:"author"`
	Committer *CommitUser `json:"committer"`
	Message   string      `json:"message"`
	Tree      *CommitMeta `json:"tree"`
}

type Commit struct {
	*CommitMeta
	HTMLURL    string        `json:"html_url"`
	RepoCommit *RepoCommit   `json:"commit"`
	Author     *User         `json:"author"`
	Committer  *User         `json:"committer"`
	Parents    []*CommitMeta `json:"parents"`
}
