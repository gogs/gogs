package conf

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/osx"
)

// cleanUpOpenSSHVersion cleans up the raw output of "ssh -V" and returns a clean version string.
func cleanUpOpenSSHVersion(raw string) string {
	v := strings.TrimRight(strings.Fields(raw)[0], ",1234567890")
	v = strings.TrimSuffix(strings.TrimPrefix(v, "OpenSSH_"), "p")
	return v
}

// openSSHVersion returns string representation of OpenSSH version via command "ssh -V".
func openSSHVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// NOTE: Somehow the version is printed to stderr.
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ssh", "-V")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrap(err, stderr.String())
	}

	return cleanUpOpenSSHVersion(stderr.String()), nil
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
// under Windows since SSH remote login is not the main method to log in on Windows.
func CheckRunUser(runUser string) (string, bool) {
	if IsWindowsRuntime() {
		return "", true
	}

	currentUser := osx.CurrentUsername()
	return currentUser, runUser == currentUser
}
