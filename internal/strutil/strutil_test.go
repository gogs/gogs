// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package strutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToUpperFirst(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		expStr string
	}{
		{
			name: "empty string",
		},
		{
			name:   "first letter is a digit",
			s:      "123 let's go",
			expStr: "123 let's go",
		},
		{
			name:   "lower to upper",
			s:      "this is a sentence",
			expStr: "This is a sentence",
		},
		{
			name:   "already in upper case",
			s:      "Let's go",
			expStr: "Let's go",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expStr, ToUpperFirst(test.s))
		})
	}
}

func TestRandomChars(t *testing.T) {
	cache := make(map[string]bool)
	for i := 0; i < 100; i++ {
		chars, err := RandomChars(10)
		if err != nil {
			t.Fatal(err)
		}
		if cache[chars] {
			t.Fatalf("Duplicated chars %q", chars)
		}
		cache[chars] = true
	}
}
