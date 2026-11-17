// Package telemetry keeps recent traces, per-minute rollups of their spans and
// measurements of the server in memory, for the admin panel.
package telemetry

import (
	"math"
	"slices"
	"time"
)

const (
	histogramMin    = 100 * time.Microsecond
	histogramGrowth = 1.1
)

var logHistogramGrowth = math.Log(histogramGrowth)

// histogram counts durations in buckets whose bounds grow by 10% from 100 µs,
// so that a percentile read from it is within about 5% of the true value. It
// only holds the buckets that have counts.
type histogram map[int]int

// histogramBucket returns the bucket of d. Bucket i holds durations above
// histogramMin×1.1^(i-1) up to histogramMin×1.1^i, and bucket 0 everything up
// to histogramMin.
func histogramBucket(d time.Duration) int {
	if d <= histogramMin {
		return 0
	}
	return int(math.Ceil(math.Log(float64(d)/float64(histogramMin)) / logHistogramGrowth))
}

// histogramValue returns the geometric middle of bucket i, which is within 5%
// of every duration in it.
func histogramValue(i int) time.Duration {
	if i == 0 {
		return histogramMin
	}
	return time.Duration(float64(histogramMin) * math.Pow(histogramGrowth, float64(i)-0.5))
}

func (h histogram) add(d time.Duration) {
	h[histogramBucket(d)]++
}

func (h histogram) merge(other histogram) {
	for i, n := range other {
		h[i] += n
	}
}

func (h histogram) count() int {
	total := 0
	for _, n := range h {
		total += n
	}
	return total
}

// quantile returns the nearest-rank q-quantile, e.g., 0.95 for p95, or zero
// when the histogram is empty.
func (h histogram) quantile(q float64) time.Duration {
	total := h.count()
	if total == 0 {
		return 0
	}
	rank := max(int(math.Ceil(q*float64(total))), 1)
	buckets := make([]int, 0, len(h))
	for i := range h {
		buckets = append(buckets, i)
	}
	slices.Sort(buckets)
	seen := 0
	for _, i := range buckets {
		seen += h[i]
		if seen >= rank {
			return histogramValue(i)
		}
	}
	return histogramValue(buckets[len(buckets)-1])
}

// below returns how many durations fall in buckets below the one of d.
func (h histogram) below(d time.Duration) int {
	bucket := histogramBucket(d)
	n := 0
	for i, c := range h {
		if i < bucket {
			n += c
		}
	}
	return n
}
