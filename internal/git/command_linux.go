package git

import (
	"syscall"
)

// setParentDeathSignal makes the kernel kill Git when Gogs dies.
func setParentDeathSignal(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGKILL
}
