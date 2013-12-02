//
// +build !windows

package cmd

import (
	"os/exec"
)

func makeLink(srcPath, destPath string) error {
	cmd := exec.Command("ln", "-s", srcPath, destPath)
	return cmd.Run()
}
