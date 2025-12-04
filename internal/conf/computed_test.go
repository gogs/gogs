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

func TestWorkDirHelper(_ *testing.T) {
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
		{env: "GOGS_WORK_DIR=/tmp", want: "/tmp"},
		{env: "", want: WorkDir()},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out, err := testutil.Exec("TestWorkDirHelper", test.env)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.want, out)
		})
	}
}

func TestCustomDirHelper(_ *testing.T) {
	if !testutil.WantHelperProcess() {
		return
	}

	fmt.Fprintln(os.Stdout, CustomDir())
}

func TestCustomDir(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{env: "GOGS_CUSTOM=/tmp", want: "/tmp"},
		{env: "", want: CustomDir()},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out, err := testutil.Exec("TestCustomDirHelper", test.env)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.want, out)
		})
	}
}

func TestHomeDirHelper(_ *testing.T) {
	if !testutil.WantHelperProcess() {
		return
	}

	fmt.Fprintln(os.Stdout, HomeDir())
}

func TestHomeDir(t *testing.T) {
	tests := []struct {
		envs []string
		want string
	}{
		{envs: []string{"HOME=/tmp"}, want: "/tmp"},
		{envs: []string{`USERPROFILE=C:\Users\Joe`}, want: `C:\Users\Joe`},
		{envs: []string{`HOMEDRIVE=C:`, `HOMEPATH=\Users\Joe`}, want: `C:\Users\Joe`},
		{envs: nil, want: ""},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out, err := testutil.Exec("TestHomeDirHelper", test.envs...)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.want, out)
		})
	}
}
