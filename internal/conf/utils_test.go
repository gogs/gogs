// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_cleanUpOpenSSHVersion(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{
			raw:  "OpenSSH_7.4p1 Ubuntu-10, OpenSSL 1.0.2g 1 Mar 2016",
			want: "7.4",
		}, {
			raw:  "OpenSSH_5.3p1, OpenSSL 1.0.1e-fips 11 Feb 2013",
			want: "5.3",
		}, {
			raw:  "OpenSSH_4.3p2, OpenSSL 0.9.8e-fips-rhel5 01 Jul 2008",
			want: "4.3",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, cleanUpOpenSSHVersion(test.raw))
		})
	}
}

func Test_ensureAbs(t *testing.T) {
	wd := WorkDir()

	tests := []struct {
		path string
		want string
	}{
		{
			path: "data/avatars",
			want: filepath.Join(wd, "data", "avatars"),
		}, {
			path: wd,
			want: wd,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, ensureAbs(test.path))
		})
	}
}
