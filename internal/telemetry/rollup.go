package telemetry

import "time"

const rollupMinutes = 60

// minuteStats aggregates the spans of one name that ended in one minute.
type minuteStats struct {
	// minute is the Unix minute the stats are for.
	minute int64
	count  int
	errors int
	total  time.Duration
	// self is the time spent in the spans themselves, without the spans they
	// started.
	self time.Duration
	hist histogram
}

// series holds the stats of the last hour, one minute at a time.
type series struct {
	minutes [rollupMinutes]minuteStats
}

func (s *series) add(end time.Time, duration, self time.Duration, failed bool) {
	minute := end.Unix() / 60
	m := &s.minutes[minute%rollupMinutes]
	if m.minute != minute || m.hist == nil {
		*m = minuteStats{minute: minute, hist: histogram{}}
	}
	m.count++
	if failed {
		m.errors++
	}
	m.total += duration
	m.self += self
	m.hist.add(duration)
}

// sum merges the stats of the Unix minutes from through to, inclusive.
func (s *series) sum(from, to int64) minuteStats {
	out := minuteStats{hist: histogram{}}
	for i := range s.minutes {
		m := &s.minutes[i]
		if m.hist == nil || m.minute < from || m.minute > to {
			continue
		}
		out.count += m.count
		out.errors += m.errors
		out.total += m.total
		out.self += m.self
		out.hist.merge(m.hist)
	}
	return out
}
