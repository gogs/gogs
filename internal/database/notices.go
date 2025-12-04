// Copyright 2023 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"
)

// NoticesStore is the storage layer for system notices.
type NoticesStore struct {
	db *gorm.DB
}

func newNoticesStore(db *gorm.DB) *NoticesStore {
	return &NoticesStore{db: db}
}

// Create creates a system notice with the given type and description.
func (s *NoticesStore) Create(ctx context.Context, typ NoticeType, desc string) error {
	return s.db.WithContext(ctx).Create(
		&Notice{
			Type:        typ,
			Description: desc,
		},
	).Error
}

// DeleteByIDs deletes system notices by given IDs.
func (s *NoticesStore) DeleteByIDs(ctx context.Context, ids ...int64) error {
	return s.db.WithContext(ctx).Where("id IN (?)", ids).Delete(&Notice{}).Error
}

// DeleteAll deletes all system notices.
func (s *NoticesStore) DeleteAll(ctx context.Context) error {
	return s.db.WithContext(ctx).Where("TRUE").Delete(&Notice{}).Error
}

// List returns a list of system notices. Results are paginated by given page
// and page size, and sorted by primary key (id) in descending order.
func (s *NoticesStore) List(ctx context.Context, page, pageSize int) ([]*Notice, error) {
	notices := make([]*Notice, 0, pageSize)
	return notices, s.db.WithContext(ctx).
		Limit(pageSize).Offset((page - 1) * pageSize).
		Order("id DESC").
		Find(&notices).
		Error
}

// Count returns the total number of system notices.
func (s *NoticesStore) Count(ctx context.Context) int64 {
	var count int64
	s.db.WithContext(ctx).Model(&Notice{}).Count(&count)
	return count
}

type NoticeType int

const (
	NoticeTypeRepository NoticeType = iota + 1
)

// TrStr returns a translation format string.
func (t NoticeType) TrStr() string {
	return "admin.notices.type_" + strconv.Itoa(int(t))
}

// Notice represents a system notice for admin.
type Notice struct {
	ID          int64 `gorm:"primaryKey"`
	Type        NoticeType
	Description string    `xorm:"TEXT" gorm:"type:TEXT"`
	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
}

// BeforeCreate implements the GORM create hook.
func (n *Notice) BeforeCreate(tx *gorm.DB) error {
	if n.CreatedUnix == 0 {
		n.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (n *Notice) AfterFind(*gorm.DB) error {
	n.Created = time.Unix(n.CreatedUnix, 0).Local()
	return nil
}

// RemoveAllWithNotice is a helper function to remove all directories in given
// path and creates a system notice in case of an error.
func RemoveAllWithNotice(title, path string) {
	if err := os.RemoveAll(path); err != nil {
		desc := fmt.Sprintf("%s [%s]: %v", title, path, err)
		if err = Handle.Notices().Create(context.Background(), NoticeTypeRepository, desc); err != nil {
			log.Error("Failed to create repository notice: %v", err)
		}
	}
}
