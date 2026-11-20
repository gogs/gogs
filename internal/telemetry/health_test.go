package telemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth_Report(t *testing.T) {
	const key = "queue"
	series := func(t *testing.T, report HealthReport) HealthSeries {
		t.Helper()
		for _, s := range report.Series {
			if s.Key == key {
				return s
			}
		}
		require.FailNow(t, "series not found")
		return HealthSeries{}
	}

	t.Run("leaves out samples from before the range", func(t *testing.T) {
		value := 0.0
		h := NewHealth(nil, Gauge{Key: key, Read: func() (float64, bool) {
			value++
			return value, true
		}})

		// Sampling paused for eight hours, so the ring still holds the samples
		// from before the pause.
		paused := testNow.Add(-8 * time.Hour)
		h.sample(paused.Add(-healthStep))
		h.sample(paused)
		h.sample(testNow.Add(-healthStep))
		h.sample(testNow)

		got := series(t, h.Report(testNow, "1h", false))
		want := [][2]float64{
			{float64(testNow.Add(-healthStep).UnixMilli()), 3},
			{float64(testNow.UnixMilli()), 4},
		}
		assert.Equal(t, want, got.Points)
	})

	t.Run("keeps the sample at the start of the range", func(t *testing.T) {
		h := NewHealth(nil, Gauge{Key: key, Read: func() (float64, bool) { return 1, true }})
		start := testNow.Add(-healthStep * healthPoints)
		h.sample(start.Add(-time.Millisecond))
		h.sample(start)

		got := series(t, h.Report(testNow, "1h", false))
		assert.Equal(t, [][2]float64{{float64(start.UnixMilli()), 1}}, got.Points)
	})
}

func TestCounter(t *testing.T) {
	var c counter
	_, _, ok := c.take(100, testNow)
	assert.False(t, ok, "the first reading has nothing to compare with")

	delta, elapsed, ok := c.take(130, testNow.Add(15*time.Second))
	require.True(t, ok)
	assert.Equal(t, 30.0, delta)
	assert.Equal(t, 15*time.Second, elapsed)

	got, ok := per(time.Minute, delta, elapsed)
	require.True(t, ok)
	assert.Equal(t, 120.0, got)
	_, ok = per(time.Minute, delta, 0)
	assert.False(t, ok)
}
