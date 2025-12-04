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
		{hostname: "localhost", want: true},       // #00
		{hostname: "127.0.0.1", want: true},       // #01
		{hostname: "::1", want: true},             // #02
		{hostname: "0:0:0:0:0:0:0:1", want: true}, // #03
		{hostname: "127.0.0.95", want: true},      // #04
		{hostname: "0.0.0.0", want: true},         // #05
		{hostname: "192.168.123.45", want: true},  // #06

		{hostname: "gogs.io", want: false},         // #07
		{hostname: "google.com", want: false},      // #08
		{hostname: "165.232.140.255", want: false}, // #09

		{hostname: "192.168.123.45", allowlist: []string{"10.0.0.17"}, want: true}, // #10
		{hostname: "gogs.local", allowlist: []string{"gogs.local"}, want: false},   // #11

		{hostname: "192.168.123.45", allowlist: []string{"*"}, want: false}, // #12
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, IsBlockedLocalHostname(test.hostname, test.allowlist))
		})
	}
}
