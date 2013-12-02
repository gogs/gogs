package cmd

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/Unknwon/com"
)

func init() {
}

func makeLink(srcPath, destPath string) error {
	// Check if Windows version is XP.
	if getWindowsVersion() >= 6 {
		cmd := exec.Command("cmd", "/c", "mklink", "/j", destPath, srcPath)
		return cmd.Run()
	}

	// XP.
	os.RemoveAll(destPath)
	return com.CopyDir(srcPath, destPath)
}

func getWindowsVersion() int {
	dll := syscall.MustLoadDLL("kernel32.dll")
	p := dll.MustFindProc("GetVersion")
	v, _, _ := p.Call()
	return int(byte(v))
}
