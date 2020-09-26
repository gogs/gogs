// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_issueReferencePattern(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		expStrings []string
	}{
		{
			name:       "no match",
			message:    "Hello world!",
			expStrings: nil,
		},
		{
			name:       "contains issue numbers",
			message:    "#123 is fixed, and #456 is WIP",
			expStrings: []string{"#123", " #456"},
		},
		{
			name:       "contains full issue references",
			message:    "#123 is fixed, and user/repo#456 is WIP",
			expStrings: []string{"#123", " user/repo#456"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			strs := issueReferencePattern.FindAllString(test.message, -1)
			assert.Equal(t, test.expStrings, strs)
		})
	}
}
