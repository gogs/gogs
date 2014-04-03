package models

import "time"

// OT: Oauth2 Type
const (
	OT_GITHUB = iota + 1
	OT_GOOGLE
	OT_TWITTER
)

type Oauth2 struct {
	Uid         int64     `xorm:"pk"`               // userId
	Type        int       `xorm:"pk unique(oauth)"` // twitter,github,google...
	Identity    string    `xorm:"pk unique(oauth)"` // id..
	Token       string    `xorm:"VARCHAR(200) not null"`
	RefreshTime time.Time `xorm:"created"`
}
