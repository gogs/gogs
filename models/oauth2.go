// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
)

// OT: Oauth2 Type
const (
	OT_GITHUB = iota + 1
	OT_GOOGLE
	OT_TWITTER
	OT_QQ
	OT_WEIBO
	OT_BITBUCKET
	OT_OSCHINA
	OT_FACEBOOK
)

var (
	ErrOauth2RecordNotExist = errors.New("OAuth2 record does not exist")
	ErrOauth2NotAssociated  = errors.New("OAuth2 is not associated with user")
)

type Oauth2 struct {
	Id       int64
	Uid      int64  `xorm:"unique(s)"` // userId
	User     *User  `xorm:"-"`
	Type     int    `xorm:"unique(s) unique(oauth)"` // twitter,github,google...
	Identity string `xorm:"unique(s) unique(oauth)"` // id..
	Token    string `xorm:"TEXT not null"`
}

func BindUserOauth2(userId, oauthId int64) error {
	_, err := orm.Id(oauthId).Update(&Oauth2{Uid: userId})
	return err
}

func AddOauth2(oa *Oauth2) error {
	_, err := orm.Insert(oa)
	return err
}

func GetOauth2(identity string) (oa *Oauth2, err error) {
	oa = &Oauth2{Identity: identity}
	isExist, err := orm.Get(oa)
	if err != nil {
		return
	} else if !isExist {
		return nil, ErrOauth2RecordNotExist
	} else if oa.Uid == -1 {
		return oa, ErrOauth2NotAssociated
	}
	oa.User, err = GetUserById(oa.Uid)
	return oa, err
}

func GetOauth2ById(id int64) (oa *Oauth2, err error) {
	oa = new(Oauth2)
	has, err := orm.Id(id).Get(oa)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrOauth2RecordNotExist
	}
	return oa, nil
}

// GetOauthByUserId returns list of oauthes that are releated to given user.
func GetOauthByUserId(uid int64) (oas []*Oauth2, err error) {
	err = orm.Find(&oas, Oauth2{Uid: uid})
	return oas, err
}
