// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriter_Print(t *testing.T) {
	tests := []struct {
		name      string
		vs        []interface{}
		expOutput string
	}{
		{
			name: "no values",
		},
		{
			name:      "only one value",
			vs:        []interface{}{"test"},
			expOutput: "test",
		},
		{
			name:      "two values",
			vs:        []interface{}{"test", "output"},
			expOutput: "testoutput",
		},

		{
			name:      "sql",
			vs:        []interface{}{"sql", "writer.go:65", "1ms", "SELECT * FROM users WHERE user_id = $1", []int{1}, 1},
			expOutput: "[sql] [writer.go:65] [1ms] SELECT * FROM users WHERE user_id = $1 [1] (1 rows affected)",
		},
		{
			name:      "log",
			vs:        []interface{}{"log", "writer.go:65", "something"},
			expOutput: "[log] [writer.go:65] something",
		},
		{
			name:      "error",
			vs:        []interface{}{"error", "writer.go:65", "something bad"},
			expOutput: "[err] [writer.go:65] something bad",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := &Writer{Writer: &buf}
			w.Print(test.vs...)
			assert.Equal(t, test.expOutput, buf.String())
		})
	}
}
