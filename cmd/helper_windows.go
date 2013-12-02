package cmd

import (
	"os/exec"
	"syscall"
)

func makeLink(oldPath, newPath string) error {
	// Check if Windows version is XP.

	cmd := exec.Command("cmd", "/c", "mklink", "/j", newPath, oldPath)
	return cmd.Run()
}

func getWindowsVersion() int {
	dll := syscall.MustLoadDLL("kernel32.dll")
	p := dll.MustFindProc("GetVersion")
	v, _, _ := p.Call()
	return v
}
