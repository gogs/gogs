// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"testing"
)

func setMockLoginSourcesStore(t *testing.T, mock LoginSourcesStore) {
	before := LoginSources
	LoginSources = mock
	t.Cleanup(func() {
		LoginSources = before
	})
}

func setMockLoginSourceFilesStore(t *testing.T, db *loginSourcesStore, mock loginSourceFilesStore) {
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

func SetMockReposStore(t *testing.T, mock ReposStore) {
	before := Repos
	Repos = mock
	t.Cleanup(func() {
		Repos = before
	})
}

func SetMockTwoFactorsStore(t *testing.T, mock TwoFactorsStore) {
	before := TwoFactors
	TwoFactors = mock
	t.Cleanup(func() {
		TwoFactors = before
	})
}

func SetMockUsersStore(t *testing.T, mock UsersStore) {
	before := Users
	Users = mock
	t.Cleanup(func() {
		Users = before
	})
}
