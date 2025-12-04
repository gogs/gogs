// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package testutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Exec executes "go test" on given helper with supplied environment variables.
// It is useful to mock "os/exec" functions in tests. When succeeded, it returns
// the result produced by the test helper.
// The test helper should:
//  1. Use WantHelperProcess function to determine if it is being called in helper mode.
//  2. Call fmt.Fprintln(os.Stdout, ...) to print results for the main test to collect.
func Exec(helper string, envs ...string) (string, error) {
	cmd := exec.Command(os.Args[0], "-test.run="+helper, "--")
	cmd.Env = []string{
		"GO_WANT_HELPER_PROCESS=1",
		"GOCOVERDIR=" + os.TempDir(),
	}
	cmd.Env = append(cmd.Env, envs...)
	out, err := cmd.CombinedOutput()
	str := string(out)

	// The error is quite confusing even when tests passed, so let's check whether
	// it is passed first.
	if strings.Contains(str, "no tests to run") {
		return "", errors.New("no tests to run")
	} else if i := strings.Index(str, "PASS"); i >= 0 {
		// Collect helper result
		return strings.TrimSpace(str[:i]), nil
	}

	if err != nil {
		return "", fmt.Errorf("%v - %s", err, str)
	}
	return "", errors.New(str)
}

// WantHelperProcess returns true if current process is in helper mode.
func WantHelperProcess() bool {
	return os.Getenv("GO_WANT_HELPER_PROCESS") == "1"
}
