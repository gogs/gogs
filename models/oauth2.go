// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"time"
)

type OauthType int

const (
	GITHUB OauthType = iota + 1
	GOOGLE
	TWITTER
	QQ
	WEIBO
	BITBUCKET
	FACEBOOK
)

var (
	ErrOauth2RecordNotExist = errors.New("OAuth2 record does not exist")
	ErrOauth2NotAssociated  = errors.New("OAuth2 is not associated with user")
)

type Oauth2 struct {
	Id                int64
	Uid               int64     `xorm:"unique(s)"` // userId
	User              *User     `xorm:"-"`
	Type              int       `xorm:"unique(s) unique(oauth)"` // twitter,github,google...
	Identity          string    `xorm:"unique(s) unique(oauth)"` // id..
	Token             string    `xorm:"TEXT not null"`
	Created           time.Time `xorm:"CREATED"`
	Updated           time.Time
	HasRecentActivity bool `xorm:"-"`
}

func BindUserOauth2(userId, oauthId int64) error {
	_, err := x.Id(oauthId).Update(&Oauth2{Uid: userId})
	return err
}

func AddOauth2(oa *Oauth2) error {
	_, err := x.Insert(oa)
	return err
}

func GetOauth2(identity string) (oa *Oauth2, err error) {
	oa = &Oauth2{Identity: identity}
	isExist, err := x.Get(oa)
	if err != nil {
		return
	} else if !isExist {
		return nil, ErrOauth2RecordNotExist
	} else if oa.Uid == -1 {
		return oa, ErrOauth2NotAssociated
	}
	oa.User, err = GetUserByID(oa.Uid)
	return oa, err
}

func GetOauth2ById(id int64) (oa *Oauth2, err error) {
	oa = new(Oauth2)
	has, err := x.Id(id).Get(oa)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrOauth2RecordNotExist
	}
	return oa, nil
}

// UpdateOauth2 updates given OAuth2.
func UpdateOauth2(oa *Oauth2) error {
	_, err := x.Id(oa.Id).AllCols().Update(oa)
	return err
}

// GetOauthByUserId returns list of oauthes that are related to given user.
func GetOauthByUserId(uid int64) ([]*Oauth2, error) {
	socials := make([]*Oauth2, 0, 5)
	err := x.Find(&socials, Oauth2{Uid: uid})
	if err != nil {
		return nil, err
	}

	for _, social := range socials {
		social.HasRecentActivity = social.Updated.Add(7 * 24 * time.Hour).After(time.Now())
	}
	return socials, err
}

// DeleteOauth2ById deletes a oauth2 by ID.
func DeleteOauth2ById(id int64) error {
	_, err := x.Delete(&Oauth2{Id: id})
	return err
}

// CleanUnbindOauth deletes all unbind OAuthes.
func CleanUnbindOauth() error {
	_, err := x.Delete(&Oauth2{Uid: -1})
	return err
}
