// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"time"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/uuid"
)

var (
	ErrAccessTokenNotExist = errors.New("Access token does not exist")
)

// AccessToken represents a personal access token.
type AccessToken struct {
	ID                int64 `xorm:"pk autoincr"`
	UID               int64 `xorm:"uid INDEX"`
	Name              string
	Sha1              string    `xorm:"UNIQUE VARCHAR(40)"`
	Created           time.Time `xorm:"CREATED"`
	Updated           time.Time
	HasRecentActivity bool `xorm:"-"`
	HasUsed           bool `xorm:"-"`
}

// NewAccessToken creates new access token.
func NewAccessToken(t *AccessToken) error {
	t.Sha1 = base.EncodeSha1(uuid.NewV4().String())
	_, err := x.Insert(t)
	return err
}

// GetAccessTokenBySHA returns access token by given sha1.
func GetAccessTokenBySHA(sha string) (*AccessToken, error) {
	t := &AccessToken{Sha1: sha}
	has, err := x.Get(t)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrAccessTokenNotExist
	}
	return t, nil
}

// ListAccessTokens returns a list of access tokens belongs to given user.
func ListAccessTokens(uid int64) ([]*AccessToken, error) {
	tokens := make([]*AccessToken, 0, 5)
	err := x.Where("uid=?", uid).Desc("id").Find(&tokens)
	if err != nil {
		return nil, err
	}

	for _, t := range tokens {
		t.HasUsed = t.Updated.After(t.Created)
		t.HasRecentActivity = t.Updated.Add(7 * 24 * time.Hour).After(time.Now())
	}
	return tokens, nil
}

// UpdateAccessToekn updates information of access token.
func UpdateAccessToekn(t *AccessToken) error {
	_, err := x.Id(t.ID).AllCols().Update(t)
	return err
}

// DeleteAccessTokenByID deletes access token by given ID.
func DeleteAccessTokenByID(id int64) error {
	_, err := x.Id(id).Delete(new(AccessToken))
	return err
}
