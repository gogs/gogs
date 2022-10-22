// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package userutil

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/tool"
)

func TestDashboardURLPath(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		got := DashboardURLPath("alice", false)
		want := "/"
		assert.Equal(t, want, got)
	})

	t.Run("organization", func(t *testing.T) {
		got := DashboardURLPath("acme", true)
		want := "/org/acme/dashboard/"
		assert.Equal(t, want, got)
	})
}

func TestGenerateActivateCode(t *testing.T) {
	conf.SetMockAuth(t,
		conf.AuthOpts{
			ActivateCodeLives: 10,
		},
	)

	code := GenerateActivateCode(1, "alice@example.com", "Alice", "123456", "rands")
	got := tool.VerifyTimeLimitCode("1alice@example.comalice123456rands", conf.Auth.ActivateCodeLives, code[:tool.TIME_LIMIT_CODE_LENGTH])
	assert.True(t, got)
}
