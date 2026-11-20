package telemetry

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// procStatus returns the number of a field of the status of the process, e.g.,
// of "Threads".
func procStatus(field string) (float64, bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name, rest, ok := strings.Cut(scanner.Text(), ":")
		if !ok || name != field {
			continue
		}
		// A size is followed by its unit, e.g., "2004 kB".
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, false
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		return value, err == nil
	}
	return 0, false
}

// processGauges returns the gauges of the CPU, memory, threads and open files
// of the process, the last against the limit of open files.
func processGauges() []Gauge {
	var openFilesLimit float64
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err == nil && rlimit.Cur < math.MaxInt64 {
		openFilesLimit = float64(rlimit.Cur)
	}
	var cpuTime counter
	return []Gauge{
		// The share of all CPUs the process used since the last sample, in
		// percent.
		{Key: "cpu", Read: func() (float64, bool) {
			var usage syscall.Rusage
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
				return 0, false
			}
			used := time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
			delta, elapsed, ok := cpuTime.take(used.Seconds(), time.Now())
			if !ok || elapsed <= 0 {
				return 0, false
			}
			return delta / elapsed.Seconds() / float64(runtime.NumCPU()) * 100, true
		}},
		{Key: "residentMemory", Read: func() (float64, bool) {
			kb, ok := procStatus("VmRSS")
			return kb * 1024, ok
		}},
		{Key: "threads", Read: func() (float64, bool) { return procStatus("Threads") }},
		{Key: "openFiles", Limit: openFilesLimit, Read: func() (float64, bool) {
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				return 0, false
			}
			return float64(len(entries)), true
		}},
	}
}

// DiskGauges returns the gauges of the used space of the file systems that
// hold the directories, against the space the server can use on them. File
// systems that hold several of the directories are measured once, under the
// key of the first.
func DiskGauges(disks ...Disk) []Gauge {
	var gauges []Gauge
	seen := make(map[uint64]bool)
	for _, disk := range disks {
		// A directory that is yet to be created is going to be on the file
		// system of the closest directory above it.
		path := disk.Path
		info, err := os.Stat(path)
		for err != nil && filepath.Dir(path) != path {
			path = filepath.Dir(path)
			info, err = os.Stat(path)
		}
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || seen[stat.Dev] {
			continue
		}
		seen[stat.Dev] = true

		// Unprivileged processes cannot use the blocks the file system
		// reserves, so those count neither as used nor as usable.
		usage := func() (used, usable float64, ok bool) {
			var fs syscall.Statfs_t
			if err := syscall.Statfs(path, &fs); err != nil {
				return 0, 0, false
			}
			used = float64(fs.Blocks-fs.Bfree) * float64(fs.Bsize)
			return used, used + float64(fs.Bavail)*float64(fs.Bsize), true
		}
		_, usable, ok := usage()
		if !ok {
			continue
		}
		gauges = append(gauges, Gauge{Key: disk.Key, Limit: usable, Read: func() (float64, bool) {
			used, _, ok := usage()
			return used, ok
		}})
	}
	return gauges
}
