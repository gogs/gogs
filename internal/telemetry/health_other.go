//go:build !linux

package telemetry

// openFilesGauge reports that open files are only measured on Linux.
func openFilesGauge() (Gauge, bool) {
	return Gauge{}, false
}
