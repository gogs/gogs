//go:build windows

package git

import (
	osexec "os/exec"
)

// killProcessGroup does nothing on Windows, where only Git itself is killed
// when the command is canceled. The wait delay still bounds how long waiting
// for the processes it started takes.
func killProcessGroup(*osexec.Cmd) {}
