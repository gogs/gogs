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

// NoticesStore is the persistent interface for system notices.
type NoticesStore interface {
	// Create creates a system notice with the given type and description.
	Create(ctx context.Context, typ NoticeType, desc string) error
	// DeleteByIDs deletes system notices by given IDs.
	DeleteByIDs(ctx context.Context, ids ...int64) error
	// DeleteAll deletes all system notices.
	DeleteAll(ctx context.Context) error
	// List returns a list of system notices. Results are paginated by given page
	// and page size, and sorted by primary key (id) in descending order.
	List(ctx context.Context, page, pageSize int) ([]*Notice, error)
	// Count returns the total number of system notices.
	Count(ctx context.Context) int64
}

var Notices NoticesStore

var _ NoticesStore = (*notices)(nil)

type notices struct {
	*gorm.DB
}

// NewNoticesStore returns a persistent interface for system notices with given
// database connection.
func NewNoticesStore(db *gorm.DB) NoticesStore {
	return &notices{DB: db}
}

func (db *notices) Create(ctx context.Context, typ NoticeType, desc string) error {
	return db.WithContext(ctx).Create(
		&Notice{
			Type:        typ,
			Description: desc,
		},
	).Error
}

func (db *notices) DeleteByIDs(ctx context.Context, ids ...int64) error {
	return db.WithContext(ctx).Where("id IN (?)", ids).Delete(&Notice{}).Error
}

func (db *notices) DeleteAll(ctx context.Context) error {
	return db.WithContext(ctx).Where("TRUE").Delete(&Notice{}).Error
}

func (db *notices) List(ctx context.Context, page, pageSize int) ([]*Notice, error) {
	notices := make([]*Notice, 0, pageSize)
	return notices, db.WithContext(ctx).
		Limit(pageSize).Offset((page - 1) * pageSize).
		Order("id DESC").
		Find(&notices).
		Error
}

func (db *notices) Count(ctx context.Context) int64 {
	var count int64
	db.WithContext(ctx).Model(&Notice{}).Count(&count)
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
	ID          int64 `gorm:"primarykey"`
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
func (n *Notice) AfterFind(_ *gorm.DB) error {
	n.Created = time.Unix(n.CreatedUnix, 0).Local()
	return nil
}

// RemoveAllWithNotice is a helper function to remove all directories in given
// path and creates a system notice in case of an error.
func RemoveAllWithNotice(title, path string) {
	if err := os.RemoveAll(path); err != nil {
		desc := fmt.Sprintf("%s [%s]: %v", title, path, err)
		if err = Notices.Create(context.Background(), NoticeTypeRepository, desc); err != nil {
			log.Error("Failed to create repository notice: %v", err)
		}
	}
}
