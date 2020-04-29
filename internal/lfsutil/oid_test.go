// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidOID(t *testing.T) {
	tests := []struct {
		name   string
		oid    OID
		expVal bool
	}{
		{
			name: "malformed",
			oid:  OID("7c222fb2927d828af22f592134e8932480637c0d"),
		},
		{
			name: "not all lower cased",
			oid:  OID("EF797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"),
		},

		{
			name:   "valid",
			oid:    OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"),
			expVal: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expVal, ValidOID(test.oid))
		})
	}
}
