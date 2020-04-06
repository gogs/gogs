// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
)

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
