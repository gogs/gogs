// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"testing"
)

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
