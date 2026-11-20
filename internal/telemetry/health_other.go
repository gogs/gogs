//go:build !linux

package telemetry

// processGauges reports that the process is only measured on Linux.
func processGauges() []Gauge {
	return nil
}

// DiskGauges reports that file systems are only measured on Linux.
func DiskGauges(...Disk) []Gauge {
	return nil
}
