//go:build !windows

package git

import (
	"os"
	osexec "os/exec"
	"syscall"
)

// killProcessGroup runs the command in its own process group and kills the
// whole group when the command is canceled, so that processes Git starts, e.g.,
// "git fetch" started by "git remote add -f" or hooks, do not outlive it.
func killProcessGroup(c *osexec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		err := syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}
