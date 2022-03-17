// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileNames(t *testing.T) {
	names, err := FileNames(".")
	require.NoError(t, err)

	want := []string{"app.ini", "auth.d", "gitignore", "label", "license", "locale", "readme"}
	assert.Equal(t, want, names)
}
