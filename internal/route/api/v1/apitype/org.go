package apitype

type Organization struct {
	ID          int64  `json:"id"`
	UserName    string `json:"username"`
	FullName    string `json:"full_name"`
	AvatarURL   string `json:"avatar_url"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

type Team struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Permission  string `json:"permission"`
}
