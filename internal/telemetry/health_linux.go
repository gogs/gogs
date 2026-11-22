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
		// The share of the CPUs the process may use that it used since the last
		// sample, in percent. Git runs in processes of its own, which this
		// leaves out. Go counts at least two CPUs for a container limited to
		// fewer, so such a server reads lower than it is.
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
			// GOMAXPROCS follows the CPU limit of a container, unlike the count
			// of CPUs of the machine, but can be set above that count.
			cpus := min(runtime.GOMAXPROCS(0), runtime.NumCPU())
			return delta / elapsed.Seconds() / float64(cpus) * 100, true
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
		if !ok {
			continue
		}
		dev := uint64(stat.Dev) //nolint:unconvert // The type of the device differs between architectures.
		if seen[dev] {
			continue
		}
		seen[dev] = true

		usage := &diskUsage{path: path, timeout: diskUsageTimeout, statfs: syscall.Statfs}
		// A file system that answers with an error is left out. One that is yet
		// to answer is measured once it does.
		if _, ok := usage.read(); !ok && usage.pending == nil {
			continue
		}
		gauges = append(gauges, Gauge{Key: disk.Key, Limit: usage.limit(), Read: usage.read, ReadLimit: usage.limit})
	}
	return gauges
}

// diskUsageTimeout is how long a file system gets to answer before a sample
// goes on without it.
const diskUsageTimeout = 2 * time.Second

// diskSpace is what a file system answered.
type diskSpace struct {
	used, usable float64
}

// diskUsage measures the file system of a path. A file system that does not
// answer in time, e.g., a network mount whose server is gone, costs a reading
// the timeout once. The readings after it do not wait, and the first of them
// to find the answer takes it. A file system that always answers late
// therefore costs the timeout once per answer, and has fewer points than the
// others. One goroutine at a time calls its methods, as Health does with its
// gauges.
type diskUsage struct {
	path    string
	timeout time.Duration
	// statfs is syscall.Statfs, which tests replace.
	statfs func(path string, fs *syscall.Statfs_t) error
	// pending receives the answer that a reading is still waiting for.
	pending chan diskSpace
	// usable is the space the server could use as of the last answer.
	usable float64
}

func (d *diskUsage) read() (used float64, ok bool) {
	waited := d.pending != nil
	if !waited {
		d.pending = d.ask()
	}
	var space diskSpace
	select {
	case space, ok = <-d.pending:
	default:
		if waited {
			return 0, false
		}
		select {
		case space, ok = <-d.pending:
		case <-time.After(d.timeout):
			return 0, false
		}
	}
	d.pending = nil
	if ok {
		d.usable = space.usable
	}
	return space.used, ok
}

// ask asks the file system, and returns the channel that receives its answer,
// or is closed without one when the file system answers with an error.
func (d *diskUsage) ask() chan diskSpace {
	answer := make(chan diskSpace, 1)
	go func() {
		defer close(answer)
		var fs syscall.Statfs_t
		if err := d.statfs(d.path, &fs); err != nil {
			return
		}
		// Blocks are counted in fragments, which some file systems, e.g., NFS,
		// size apart from the block size.
		size := float64(fs.Frsize)
		if size <= 0 {
			size = float64(fs.Bsize)
		}
		// Unprivileged processes cannot use the blocks the file system
		// reserves, so those count neither as used nor as usable.
		used := float64(fs.Blocks-fs.Bfree) * size
		answer <- diskSpace{used: used, usable: used + float64(fs.Bavail)*size}
	}()
	return answer
}

func (d *diskUsage) limit() float64 {
	return d.usable
}
