// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"time"
	"strings"
	"fmt"
)

const (
	PROTECTED_BRANCH_USER_ID = "__UER_ID"
	PROTECTED_BRANCH_REPO_ID = "__REPO_ID"
	PROTECTED_BRANCH_ACCESS_MODE = "__ACCESS_MODE"
)
type  ProtectedBranch struct {
	ID          int64 `xorm:"pk autoincr"`
	RepoID      int64`xorm:"UNIQUE(s)"`
	BranchName  string`xorm:"UNIQUE(s)"`
	CanPush     bool
	Created     time.Time `xorm:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-"`
	UpdatedUnix int64
}

func (protectBranch *ProtectedBranch) BeforeInsert() {
	protectBranch.CreatedUnix = time.Now().Unix()
	protectBranch.UpdatedUnix = protectBranch.CreatedUnix
}

func (protectBranch *ProtectedBranch) BeforeUpdate() {
	protectBranch.UpdatedUnix = time.Now().Unix()
}
func GetProtectedBranchByRepoID(RepoID int64) ([] *ProtectedBranch, error) {
	protectedBranches := make([]*ProtectedBranch, 0)
	return protectedBranches, x.Where("repo_id = ?", RepoID).Desc("updated_unix").Find(&protectedBranches)
}
func GetProtectedBranchBy(repoID int64, BranchName string) (*ProtectedBranch, error) {
	rel := &ProtectedBranch{RepoID: repoID, BranchName: strings.ToLower(BranchName)}
	_, err := x.Get(rel)
	return rel, err
}

func (repo *Repository) GetProtectedBranches() ([]*ProtectedBranch, error) {
	protectedBranches := make([]*ProtectedBranch, 0)
	return protectedBranches, x.Find(&protectedBranches, &ProtectedBranch{RepoID: repo.ID})
}

func (repo *Repository) AddProtectedBranch(branchName string, canPush bool) error {
	protectedBranch := &ProtectedBranch{
		RepoID: repo.ID,
		BranchName: branchName,
	}

	has, err := x.Get(protectedBranch)
	if err != nil {
		return err
	} else if has {
		return nil
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}
	protectedBranch.CanPush = canPush
	if _, err = sess.InsertOne(protectedBranch); err != nil {
		return err
	}

	return sess.Commit()
}

// ChangeProtectedBranchAccessMode sets new access mode for the ProtectedBranch.
func (repo *Repository) ChangeProtectedBranch(id int64, canPush bool) error {
	ProtectedBranch := &ProtectedBranch{
		RepoID: repo.ID,
		ID: id,
	}
	has, err := x.Get(ProtectedBranch)
	if err != nil {
		return fmt.Errorf("get ProtectedBranch: %v", err)
	} else if !has {
		return nil
	}

	if ProtectedBranch.CanPush == canPush {
		return nil
	}
	ProtectedBranch.CanPush = canPush

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Id(ProtectedBranch.ID).AllCols().Update(ProtectedBranch); err != nil {
		return fmt.Errorf("update ProtectedBranch: %v", err)
	}

	return sess.Commit()
}

// DeleteProtectedBranch removes ProtectedBranch relation between the user and repository.
func (repo *Repository) DeleteProtectedBranch(id int64) (err error) {
	ProtectedBranch := &ProtectedBranch{
		RepoID: repo.ID,
		ID: id,
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if has, err := sess.Delete(ProtectedBranch); err != nil || has == 0 {
		return err
	}

	return sess.Commit()
}

func newProtectedBranch(protectedBranch *ProtectedBranch) error {
	_, err := x.InsertOne(protectedBranch)
	return err
}

func UpdateProtectedBranch(protectedBranch *ProtectedBranch) error {
	_, err := x.Update(protectedBranch)
	return err
}
