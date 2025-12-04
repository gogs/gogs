// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
)

func TestQuote(t *testing.T) {
	conf.UsePostgreSQL = true
	got := Quote("SELECT * FROM %s", "user")
	want := `SELECT * FROM "user"`
	assert.Equal(t, want, got)
	conf.UsePostgreSQL = false

	got = Quote("SELECT * FROM %s", "user")
	want = `SELECT * FROM user`
	assert.Equal(t, want, got)
}
