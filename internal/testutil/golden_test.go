// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package testutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdate(t *testing.T) {
	before := updateRegex
	defer func() {
		updateRegex = before
	}()

	t.Run("no flag", func(t *testing.T) {
		updateRegex = nil
		assert.False(t, Update("TestUpdate"))
	})

	tests := []struct {
		regex string
		name  string
		want  bool
	}{
		{regex: "", name: "TestUpdate", want: false},
		{regex: "TestNotFound", name: "TestUpdate", want: false},

		{regex: ".*", name: "TestUpdate", want: true},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			updateRegex = &test.regex
			assert.Equal(t, test.want, Update(test.name))
		})
	}
}

func TestAssertGolden(t *testing.T) {
	// Make sure it does not blow up
	AssertGolden(t, filepath.Join("testdata", "golden"), false, "{\n  \"Message\": \"This is a golden file.\"\n}")
	AssertGolden(t, filepath.Join("testdata", "golden"), false, []byte("{\n  \"Message\": \"This is a golden file.\"\n}"))

	type T struct {
		Message string
	}
	AssertGolden(t, filepath.Join("testdata", "golden"), false, T{"This is a golden file."})
}
