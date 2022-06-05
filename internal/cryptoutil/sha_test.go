// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cryptoutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSHA1(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{input: "", output: "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{input: "The quick brown fox jumps over the lazy dog", output: "2fd4e1c67a2d28fced849ee1bb76e7391b93eb12"},
		{input: "The quick brown fox jumps over the lazy dog.", output: "408d94384216f890ff7a0c3528e8bed1e0b01621"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.output, SHA1(test.input))
		})
	}
}

func TestSHA256(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{input: "", output: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{input: "The quick brown fox jumps over the lazy dog", output: "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592"},
		{input: "The quick brown fox jumps over the lazy dog.", output: "ef537f25c895bfa782526529a9b63d97aa631564d5d789c2b765448c8635fb6c"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.output, SHA256(test.input))
		})
	}
}
