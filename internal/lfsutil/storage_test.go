// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStorageLocalPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	tests := []struct {
		name    string
		root    string
		oid     OID
		expPath string
	}{
		{
			name: "invalid oid",
			oid:  OID("bad_oid"),
		},

		{
			name:    "valid oid",
			root:    "/lfs-objects",
			oid:     OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"),
			expPath: "/lfs-objects/e/f/ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expPath, StorageLocalPath(test.root, test.oid))
		})
	}
}
