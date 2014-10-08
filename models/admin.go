// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"time"

	"github.com/Unknwon/com"
)

type NoticeType int

const (
	NOTICE_REPOSITORY NoticeType = iota + 1
)

// Notice represents a system notice for admin.
type Notice struct {
	Id          int64
	Type        NoticeType
	Description string    `xorm:"TEXT"`
	Created     time.Time `xorm:"CREATED"`
}

// TrStr returns a translation format string.
func (n *Notice) TrStr() string {
	return "admin.notices.type_" + com.ToStr(n.Type)
}

// CreateNotice creates new system notice.
func CreateNotice(tp NoticeType, desc string) error {
	n := &Notice{
		Type:        tp,
		Description: desc,
	}
	_, err := x.Insert(n)
	return err
}

// CreateRepositoryNotice creates new system notice with type NOTICE_REPOSITORY.
func CreateRepositoryNotice(desc string) error {
	return CreateNotice(NOTICE_REPOSITORY, desc)
}

// CountNotices returns number of notices.
func CountNotices() int64 {
	count, _ := x.Count(new(Notice))
	return count
}

// GetNotices returns given number of notices with offset.
func GetNotices(num, offset int) ([]*Notice, error) {
	notices := make([]*Notice, 0, num)
	err := x.Limit(num, offset).Desc("id").Find(&notices)
	return notices, err
}

// DeleteNotice deletes a system notice by given ID.
func DeleteNotice(id int64) error {
	_, err := x.Id(id).Delete(new(Notice))
	return err
}
