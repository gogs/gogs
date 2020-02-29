// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/process"
)

// cleanUpOpenSSHVersion cleans up the raw output of "ssh -V" and returns a clean version string.
func cleanUpOpenSSHVersion(raw string) string {
	v := strings.TrimRight(strings.Fields(raw)[0], ",1234567890")
	v = strings.TrimSuffix(strings.TrimPrefix(v, "OpenSSH_"), "p")
	return v
}

// openSSHVersion returns string representation of OpenSSH version via command "ssh -V".
func openSSHVersion() (string, error) {
	// NOTE: Somehow the version is printed to stderr.
	_, stderr, err := process.Exec("conf.openSSHVersion", "ssh", "-V")
	if err != nil {
		return "", errors.Wrap(err, stderr)
	}

	return cleanUpOpenSSHVersion(stderr), nil
}

// ensureAbs prepends the WorkDir to the given path if it is not an absolute path.
func ensureAbs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(WorkDir(), path)
}

// CheckRunUser returns false if configured run user does not match actual user that
// runs the app. The first return value is the actual user name. This check is ignored
// under Windows since SSH remote login is not the main method to login on Windows.
func CheckRunUser(runUser string) (string, bool) {
	if IsWindowsRuntime() {
		return "", true
	}

	currentUser := osutil.CurrentUsername()
	return currentUser, runUser == currentUser
}
