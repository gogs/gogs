package telemetry

import (
	"path/filepath"
	"testing"

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
	used, ok := gauge.Read()
	require.True(t, ok)
	assert.Positive(t, used)
	assert.GreaterOrEqual(t, gauge.ReadLimit(), used)
	assert.Equal(t, gauge.Limit, gauge.ReadLimit(), "the size of the file system is the same between two readings")
}

func TestDiskUsage_SkipsWhileBusy(t *testing.T) {
	usage := &diskUsage{path: t.TempDir()}
	usage.busy.Store(true)
	_, ok := usage.read()
	assert.False(t, ok, "a reading that is still waiting for the file system is not started again")

	usage.busy.Store(false)
	_, ok = usage.read()
	assert.True(t, ok)
}
