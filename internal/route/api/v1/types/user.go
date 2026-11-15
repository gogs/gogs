package types

import "time"

type User struct {
	ID        int64  `json:"id"`
	UserName  string `json:"username"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// AdminUser is a user with the account settings that only site admins see.
type AdminUser struct {
	User
	IsAdmin          bool      `json:"is_admin"`
	IsActive         bool      `json:"is_active"`
	ProhibitLogin    bool      `json:"prohibit_login"`
	SourceID         int64     `json:"source_id"`
	LoginName        string    `json:"login_name"`
	Website          string    `json:"website"`
	Location         string    `json:"location"`
	AllowGitHook     bool      `json:"allow_git_hook"`
	AllowImportLocal bool      `json:"allow_import_local"`
	MaxRepoCreation  int       `json:"max_repo_creation"`
	NumRepos         int       `json:"num_repos"`
	Created          time.Time `json:"created_at"`
	Updated          time.Time `json:"updated_at"`
}

type UserEmail struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
	Primary  bool   `json:"primary"`
}

type UserAccessToken struct {
	Name string `json:"name"`
	Sha1 string `json:"sha1"`
}

type UserPublicKey struct {
	ID      int64     `json:"id"`
	Key     string    `json:"key"`
	URL     string    `json:"url,omitempty"`
	Title   string    `json:"title,omitempty"`
	Created time.Time `json:"created_at"`
}

type RepositoryCollaborator struct {
	*User
	Permissions RepositoryPermission `json:"permissions"`
}
