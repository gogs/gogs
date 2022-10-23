// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// FollowsStore is the persistent interface for follows.
//
// NOTE: All methods are sorted in alphabetical order.
type FollowsStore interface {
	// Follow marks the user to follow the other user.
	Follow(ctx context.Context, userID, followID int64) error
	// IsFollowing returns true if the user is following the other user.
	IsFollowing(ctx context.Context, userID, followID int64) bool
	// Unfollow removes the mark the user to follow the other user.
	Unfollow(ctx context.Context, userID, followID int64) error
}

var Follows FollowsStore

var _ FollowsStore = (*follows)(nil)

type follows struct {
	*gorm.DB
}

// NewFollowsStore returns a persistent interface for follows with given
// database connection.
func NewFollowsStore(db *gorm.DB) FollowsStore {
	return &follows{DB: db}
}

func (*follows) updateFollowingCount(tx *gorm.DB, userID, followID int64) error {
	/*
		Equivalent SQL for PostgreSQL:

		UPDATE "user"
		SET num_followers = (
			SELECT COUNT(*) FROM follow WHERE follow_id = @followID
		)
		WHERE id = @followID
	*/
	err := tx.Model(&User{}).
		Where("id = ?", followID).
		Update(
			"num_followers",
			tx.Model(&Follow{}).Select("COUNT(*)").Where("follow_id = ?", followID),
		).
		Error
	if err != nil {
		return errors.Wrap(err, `update "num_followers"`)
	}

	/*
		Equivalent SQL for PostgreSQL:

		UPDATE "user"
		SET num_following = (
			SELECT COUNT(*) FROM follow WHERE user_id = @userID
		)
		WHERE id = @userID
	*/
	err = tx.Model(&User{}).
		Where("id = ?", userID).
		Update(
			"num_following",
			tx.Model(&Follow{}).Select("COUNT(*)").Where("user_id = ?", userID),
		).
		Error
	if err != nil {
		return errors.Wrap(err, `update "num_following"`)
	}
	return nil
}

func (db *follows) Follow(ctx context.Context, userID, followID int64) error {
	if userID == followID {
		return nil
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		f := &Follow{
			UserID:   userID,
			FollowID: followID,
		}
		result := tx.FirstOrCreate(f, f)
		if result.Error != nil {
			return errors.Wrap(result.Error, "upsert")
		} else if result.RowsAffected <= 0 {
			return nil // Relation already exists
		}

		return db.updateFollowingCount(tx, userID, followID)
	})
}

func (db *follows) IsFollowing(ctx context.Context, userID, followID int64) bool {
	return db.WithContext(ctx).Where("user_id = ? AND follow_id = ?", userID, followID).First(&Follow{}).Error == nil
}

func (db *follows) Unfollow(ctx context.Context, userID, followID int64) error {
	if userID == followID {
		return nil
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_id = ? AND follow_id = ?", userID, followID).Delete(&Follow{}).Error
		if err != nil {
			return errors.Wrap(err, "delete")
		}
		return db.updateFollowingCount(tx, userID, followID)
	})
}

// Follow represents relations of users and their followers.
type Follow struct {
	ID       int64 `gorm:"primaryKey"`
	UserID   int64 `xorm:"UNIQUE(follow)" gorm:"uniqueIndex:follow_user_follow_unique;not null"`
	FollowID int64 `xorm:"UNIQUE(follow)" gorm:"uniqueIndex:follow_user_follow_unique;not null"`
}
