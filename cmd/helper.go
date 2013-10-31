//
// +build !windows

package cmd

import (
	"os/exec"
)

func makeLink(oldPath, newPath string) error {
	cmd := exec.Command("ln", "-s", oldPath, newPath)
	return cmd.Run()
}
