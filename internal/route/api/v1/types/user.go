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
	Created time.Time `json:"created_at,omitempty"`
}

type RepositoryCollaborator struct {
	*User
	Permissions RepositoryPermission `json:"permissions"`
}
