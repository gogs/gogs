package git

import (
	"syscall"
)

// setParentDeathSignal makes the kernel kill Git when Gogs dies without the
// chance to call KillAll, e.g., when killed with SIGKILL.
func setParentDeathSignal(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGKILL
}
