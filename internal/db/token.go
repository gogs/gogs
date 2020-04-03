// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"time"

	gouuid "github.com/satori/go.uuid"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/tool"
)

// AccessToken represents a personal access token.
type AccessToken struct {
	ID     int64
	UserID int64 `xorm:"uid INDEX" gorm:"COLUMN:uid"`
	Name   string
	Sha1   string `xorm:"UNIQUE VARCHAR(40)"`

	Created           time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix       int64
	Updated           time.Time `xorm:"-" gorm:"-" json:"-"` // Note: Updated must below Created for AfterSet.
	UpdatedUnix       int64
	HasRecentActivity bool `xorm:"-" gorm:"-" json:"-"`
	HasUsed           bool `xorm:"-" gorm:"-" json:"-"`
}

func (t *AccessToken) BeforeInsert() {
	t.CreatedUnix = time.Now().Unix()
}

func (t *AccessToken) BeforeUpdate() {
	t.UpdatedUnix = time.Now().Unix()
}

func (t *AccessToken) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		t.Created = time.Unix(t.CreatedUnix, 0).Local()
	case "updated_unix":
		t.Updated = time.Unix(t.UpdatedUnix, 0).Local()
		t.HasUsed = t.Updated.After(t.Created)
		t.HasRecentActivity = t.Updated.Add(7 * 24 * time.Hour).After(time.Now())
	}
}

// NewAccessToken creates new access token.
func NewAccessToken(t *AccessToken) error {
	t.Sha1 = tool.SHA1(gouuid.NewV4().String())
	has, err := x.Get(&AccessToken{
		UserID: t.UserID,
		Name:   t.Name,
	})
	if err != nil {
		return err
	} else if has {
		return errors.AccessTokenNameAlreadyExist{Name: t.Name}
	}

	_, err = x.Insert(t)
	return err
}

// ListAccessTokens returns a list of access tokens belongs to given user.
func ListAccessTokens(uid int64) ([]*AccessToken, error) {
	tokens := make([]*AccessToken, 0, 5)
	return tokens, x.Where("uid=?", uid).Desc("id").Find(&tokens)
}

// DeleteAccessTokenOfUserByID deletes access token by given ID.
func DeleteAccessTokenOfUserByID(userID, id int64) error {
	_, err := x.Delete(&AccessToken{
		ID:     id,
		UserID: userID,
	})
	return err
}
