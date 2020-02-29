// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package testutil

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecHelper(t *testing.T) {
	if !WantHelperProcess() {
		return
	}

	if os.Getenv("PASS") != "1" {
		fmt.Fprintln(os.Stdout, "tests failed")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stdout, "tests succeed")
}

func TestExec(t *testing.T) {
	tests := []struct {
		helper string
		env    string
		expOut string
		expErr error
	}{
		{
			helper: "NoTestsToRun",
			expErr: errors.New("no tests to run"),
		}, {
			helper: "TestExecHelper",
			expErr: errors.New("exit status 1 - tests failed\n"),
		}, {
			helper: "TestExecHelper",
			env:    "PASS=1",
			expOut: "tests succeed",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			out, err := Exec(test.helper, test.env)
			assert.Equal(t, test.expErr, err)
			assert.Equal(t, test.expOut, out)
		})
	}
}
