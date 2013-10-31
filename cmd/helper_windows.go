package cmd

import (
	"os/exec"
)

func makeLink(oldPath, newPath string) error {
	cmd := exec.Command("cmd", "/c", "mklink", "/j", newPath, oldPath)
	return cmd.Run()
}
