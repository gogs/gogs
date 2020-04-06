// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
)

// NOTE: Mocks are sorted in alphabetical order.

var _ AccessTokensStore = (*MockAccessTokensStore)(nil)

type MockAccessTokensStore struct {
	MockGetBySHA func(sha string) (*AccessToken, error)
	MockSave     func(t *AccessToken) error
}

func (m *MockAccessTokensStore) GetBySHA(sha string) (*AccessToken, error) {
	return m.MockGetBySHA(sha)
}

func (m *MockAccessTokensStore) Save(t *AccessToken) error {
	return m.MockSave(t)
}

func SetMockAccessTokensStore(t *testing.T, mock AccessTokensStore) {
	before := AccessTokens
	AccessTokens = mock
	t.Cleanup(func() {
		AccessTokens = before
	})
}

var _ PermsStore = (*MockPermsStore)(nil)

type MockPermsStore struct {
	MockAccessMode func(userID int64, repo *Repository) AccessMode
	MockAuthorize  func(userID int64, repo *Repository, desired AccessMode) bool
}

func (m *MockPermsStore) AccessMode(userID int64, repo *Repository) AccessMode {
	return m.MockAccessMode(userID, repo)
}

func (m *MockPermsStore) Authorize(userID int64, repo *Repository, desired AccessMode) bool {
	return m.MockAuthorize(userID, repo, desired)
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
	MockIsUserEnabled func(userID int64) bool
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
	MockGetByID       func(id int64) (*User, error)
	MockGetByUsername func(username string) (*User, error)
}

func (m *MockUsersStore) Authenticate(username, password string, loginSourceID int64) (*User, error) {
	return m.MockAuthenticate(username, password, loginSourceID)
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
