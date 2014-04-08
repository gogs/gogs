package models

import "errors"

// OT: Oauth2 Type
const (
	OT_GITHUB = iota + 1
	OT_GOOGLE
	OT_TWITTER
)

var (
	ErrOauth2RecordNotExists       = errors.New("not exists oauth2 record")
	ErrOauth2NotAssociatedWithUser = errors.New("not associated with user")
)

type Oauth2 struct {
	Id       int64
	Uid      int64  `xorm:"pk"` // userId
	User     *User  `xorm:"-"`
	Type     int    `xorm:"pk unique(oauth)"` // twitter,github,google...
	Identity string `xorm:"pk unique(oauth)"` // id..
	Token    string `xorm:"VARCHAR(200) not null"`
}

func AddOauth2(oa *Oauth2) (err error) {
	if _, err = orm.Insert(oa); err != nil {
		return err
	}
	return nil
}

func GetOauth2(identity string) (oa *Oauth2, err error) {
	oa = &Oauth2{}
	oa.Identity = identity
	exists, err := orm.Get(oa)
	if err != nil {
		return
	}
	if !exists {
		return nil, ErrOauth2RecordNotExists
	}
	if oa.Uid == 0 {
		return oa, ErrOauth2NotAssociatedWithUser
	}
	oa.User, err = GetUserById(oa.Uid)
	return
}
