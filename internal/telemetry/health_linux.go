package telemetry

import (
	"math"
	"os"
	"syscall"
)

// openFilesGauge returns the gauge of the files the process has open, against
// the limit of open files.
func openFilesGauge() (Gauge, bool) {
	var limit float64
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err == nil && rlimit.Cur < math.MaxInt64 {
		limit = float64(rlimit.Cur)
	}
	return Gauge{Key: "openFiles", Limit: limit, Read: func() (float64, bool) {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			return 0, false
		}
		return float64(len(entries)), true
	}}, true
}
