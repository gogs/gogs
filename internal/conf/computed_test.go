// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/testutil"
)

func TestIsProdMode(t *testing.T) {
	before := App.RunMode
	defer func() {
		App.RunMode = before
	}()

	tests := []struct {
		mode string
		want bool
	}{
		{mode: "dev", want: false},
		{mode: "test", want: false},

		{mode: "prod", want: true},
		{mode: "Prod", want: true},
		{mode: "PROD", want: true},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			App.RunMode = test.mode
			assert.Equal(t, test.want, IsProdMode())
		})
	}
}

func TestWorkDirHelper(t *testing.T) {
	if !testutil.WantHelperProcess() {
		return
	}

	fmt.Fprintln(os.Stdout, WorkDir())
}

func TestWorkDir(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{env: "/tmp", want: "/tmp"},
		{env: "", want: WorkDir()},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out, err := testutil.Exec("TestWorkDirHelper", "GOGS_WORK_DIR="+test.env)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.want, out)
		})
	}
}
