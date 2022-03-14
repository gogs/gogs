// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package netutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalHostname(t *testing.T) {
	tests := []struct {
		hostname  string
		allowlist []string
		want      bool
	}{
		{hostname: "localhost", want: true},
		{hostname: "127.0.0.1", want: true},
		{hostname: "::1", want: true},
		{hostname: "0:0:0:0:0:0:0:1", want: true},
		{hostname: "fuf.me", want: true},
		{hostname: "127.0.0.95", want: true},
		{hostname: "0.0.0.0", want: true},
		{hostname: "192.168.123.45", want: true},

		{hostname: "gogs.io", want: false},
		{hostname: "google.com", want: false},
		{hostname: "165.232.140.255", want: false},

		{hostname: "192.168.123.45", allowlist: []string{"10.0.0.17"}, want: true},
		{hostname: "gogs.local", allowlist: []string{"gogs.local"}, want: false},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, IsLocalHostname(test.hostname, test.allowlist))
		})
	}
}
