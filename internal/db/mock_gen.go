// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
)

//go:generate go-mockgen -f gogs.io/gogs/internal/db -i AccessTokensStore -i LFSStore -i LoginSourcesStore -i LoginSourceFilesStore -i loginSourceFileStore -i PermsStore -o mocks.go

func SetMockAccessTokensStore(t *testing.T, mock AccessTokensStore) {
	before := AccessTokens
	AccessTokens = mock
	t.Cleanup(func() {
		AccessTokens = before
	})
}

func SetMockLFSStore(t *testing.T, mock LFSStore) {
	before := LFS
	LFS = mock
	t.Cleanup(func() {
		LFS = before
	})
}

func setMockLoginSourceFilesStore(t *testing.T, db *loginSources, mock loginSourceFilesStore) {
	before := db.files
	db.files = mock
	t.Cleanup(func() {
		db.files = before
	})
}

func SetMockPermsStore(t *testing.T, mock PermsStore) {
	before := Perms
	Perms = mock
	t.Cleanup(func() {
		Perms = before
	})
}

var _ ReposStore = (*MockReposStore)(nil)

type MockReposStore struct {
	MockGetByName func(ownerID int64, name string) (*Repository, error)
}

func (m *MockReposStore) GetByName(ownerID int64, name string) (*Repository, error) {
	return m.MockGetByName(ownerID, name)
}

func SetMockReposStore(t *testing.T, mock ReposStore) {
	before := Repos
	Repos = mock
	t.Cleanup(func() {
		Repos = before
	})
}

var _ TwoFactorsStore = (*MockTwoFactorsStore)(nil)

type MockTwoFactorsStore struct {
	MockCreate        func(userID int64, key, secret string) error
	MockGetByUserID   func(userID int64) (*TwoFactor, error)
	MockIsUserEnabled func(userID int64) bool
}

func (m *MockTwoFactorsStore) Create(userID int64, key, secret string) error {
	return m.MockCreate(userID, key, secret)
}

func (m *MockTwoFactorsStore) GetByUserID(userID int64) (*TwoFactor, error) {
	return m.MockGetByUserID(userID)
}

func (m *MockTwoFactorsStore) IsUserEnabled(userID int64) bool {
	return m.MockIsUserEnabled(userID)
}

func SetMockTwoFactorsStore(t *testing.T, mock TwoFactorsStore) {
	before := TwoFactors
	TwoFactors = mock
	t.Cleanup(func() {
		TwoFactors = before
	})
}

var _ UsersStore = (*MockUsersStore)(nil)

type MockUsersStore struct {
	MockAuthenticate  func(username, password string, loginSourceID int64) (*User, error)
	MockCreate        func(username, email string, opts CreateUserOpts) (*User, error)
	MockGetByEmail    func(email string) (*User, error)
	MockGetByID       func(id int64) (*User, error)
	MockGetByUsername func(username string) (*User, error)
}

func (m *MockUsersStore) Authenticate(username, password string, loginSourceID int64) (*User, error) {
	return m.MockAuthenticate(username, password, loginSourceID)
}

func (m *MockUsersStore) Create(username, email string, opts CreateUserOpts) (*User, error) {
	return m.MockCreate(username, email, opts)
}

func (m *MockUsersStore) GetByEmail(email string) (*User, error) {
	return m.MockGetByEmail(email)
}

func (m *MockUsersStore) GetByID(id int64) (*User, error) {
	return m.MockGetByID(id)
}

func (m *MockUsersStore) GetByUsername(username string) (*User, error) {
	return m.MockGetByUsername(username)
}

func SetMockUsersStore(t *testing.T, mock UsersStore) {
	before := Users
	Users = mock
	t.Cleanup(func() {
		Users = before
	})
}
