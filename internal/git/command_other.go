//go:build !linux && !windows

package git

import (
	"syscall"
)

// setParentDeathSignal does nothing on systems without a parent death signal.
func setParentDeathSignal(*syscall.SysProcAttr) {}
