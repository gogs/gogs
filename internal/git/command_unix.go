//go:build !windows

package git

import (
	"os"
	osexec "os/exec"
	"syscall"
)

// processTree is the process group that the command runs in.
type processTree struct{}

// newProcessTree runs the command in its own process group and kills the
// whole group when the command is canceled, so that processes Git starts, e.g.,
// "git fetch" started by "git remote add -f" or hooks, do not outlive it.
func newProcessTree(c *osexec.Cmd) (*processTree, error) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	setParentDeathSignal(c.SysProcAttr)
	c.Cancel = func() error {
		err := syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	return &processTree{}, nil
}

// attach does nothing, because the process group is set up when the command
// starts.
func (*processTree) attach(*osexec.Cmd) error { return nil }

func (*processTree) close() {}
