package telemetry

import (
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskGauges(t *testing.T) {
	dir := t.TempDir()
	gauges := DiskGauges(
		Disk{Key: "first", Path: filepath.Join(dir, "yet", "to", "be", "created")},
		Disk{Key: "second", Path: dir},
	)
	require.Len(t, gauges, 1, "a file system that holds two of the directories is measured once")

	gauge := gauges[0]
	assert.Equal(t, "first", gauge.Key)
	assert.Positive(t, gauge.Limit)
	// Readings right after each other each get an answer.
	for range 100 {
		used, ok := gauge.Read()
		require.True(t, ok)
		assert.Positive(t, used)
		assert.GreaterOrEqual(t, gauge.ReadLimit(), used)
	}
}

func TestDiskUsage(t *testing.T) {
	answer := func(fs *syscall.Statfs_t, blocks, free, available uint64) {
		fs.Bsize, fs.Frsize = 4096, 1024
		fs.Blocks, fs.Bfree, fs.Bavail = blocks, free, available
	}

	t.Run("counts blocks in fragments and leaves out the reserved ones", func(t *testing.T) {
		usage := &diskUsage{timeout: time.Minute, statfs: func(_ string, fs *syscall.Statfs_t) error {
			answer(fs, 100, 40, 30)
			return nil
		}}
		used, ok := usage.read()
		require.True(t, ok)
		assert.Equal(t, 60.0*1024, used)
		assert.Equal(t, 90.0*1024, usage.limit())
	})

	t.Run("an error is not a measurement", func(t *testing.T) {
		usage := &diskUsage{timeout: time.Minute, statfs: func(string, *syscall.Statfs_t) error {
			return syscall.EIO
		}}
		_, ok := usage.read()
		assert.False(t, ok)
		assert.Nil(t, usage.pending, "the next reading asks again")
	})

	t.Run("waits for a file system that hangs only once", func(t *testing.T) {
		var asked atomic.Int32
		release := make(chan struct{})
		usage := &diskUsage{timeout: 10 * time.Millisecond, statfs: func(_ string, fs *syscall.Statfs_t) error {
			if asked.Add(1) == 1 {
				<-release
			}
			answer(fs, 100, 40, 30)
			return nil
		}}

		_, ok := usage.read()
		require.False(t, ok, "the first reading gives up after the timeout")

		// A reading that waited again would never return within a minute.
		usage.timeout = time.Minute
		_, ok = usage.read()
		require.False(t, ok)
		assert.Equal(t, int32(1), asked.Load(), "the file system is not asked again while it is yet to answer")

		close(release)
		var used float64
		require.Eventually(t, func() bool {
			used, ok = usage.read()
			return ok
		}, 5*time.Second, time.Millisecond, "a later reading takes the answer")
		assert.Equal(t, 60.0*1024, used)
		assert.Equal(t, int32(1), asked.Load())

		_, ok = usage.read()
		assert.True(t, ok)
		assert.Equal(t, int32(2), asked.Load(), "the reading after the answer asks again")
	})
}
