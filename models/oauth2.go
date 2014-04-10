package models

import "fmt"

// OT: Oauth2 Type
const (
	OT_GITHUB = iota + 1
	OT_GOOGLE
	OT_TWITTER
)

type Oauth2 struct {
	Uid      int64  `xorm:"pk"`               // userId
	Type     int    `xorm:"pk unique(oauth)"` // twitter,github,google...
	Identity string `xorm:"pk unique(oauth)"` // id..
	Token    string `xorm:"VARCHAR(200) not null"`
	//RefreshTime time.Time `xorm:"created"`
}

func AddOauth2(oa *Oauth2) (err error) {
	if _, err = orm.Insert(oa); err != nil {
		return err
	}
	return nil
}

func GetOauth2User(identity string) (u *User, err error) {
	oa := &Oauth2{}
	oa.Identity = identity
	exists, err := orm.Get(oa)
	if err != nil {
		return
	}
	if !exists {
		err = fmt.Errorf("not exists oauth2: %s", identity)
		return
	}
	return GetUserById(oa.Uid)
}
