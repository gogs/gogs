package model

import (
	"math"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// minScheduleInterval is the shortest interval at which GitHub runs a
// scheduled workflow.
const minScheduleInterval = 5 * time.Minute

// Cron is a schedule of the schedule event, ready to compute when it fires.
type Cron struct {
	minute, hour, dom, month, dow uint64 // Bit i is set when value i matches.
	// POSIX matches a day when either the day of the month or the day of the
	// week matches, if both are restricted, that is, neither is "*".
	domRestricted, dowRestricted bool
	location                     *time.Location
}

// ParseSchedule parses a schedule that Load accepted. Like GitHub, it
// reads the cron expression in POSIX syntax, in the IANA time zone of the
// schedule or in UTC.
func ParseSchedule(s Schedule) (*Cron, error) {
	if !ValidCron(s.Cron) {
		return nil, errors.Newf("invalid cron expression %q", s.Cron)
	}
	c := &Cron{location: time.UTC}
	if s.Timezone != "" {
		// Go reads "Local" as the time zone of the server, which is not an
		// IANA name.
		loc, err := time.LoadLocation(s.Timezone)
		if err != nil || s.Timezone == "Local" {
			return nil, errors.Newf("invalid time zone %q", s.Timezone)
		}
		c.location = loc
	}
	fields := strings.Fields(s.Cron)
	bits := []*uint64{&c.minute, &c.hour, &c.dom, &c.month, &c.dow}
	for i, r := range []cronRange{minuteRange, hourRange, domRange, monthRange, dowRange} {
		v, err := r.parse(fields[i])
		if err != nil {
			return nil, errors.Wrapf(err, "cron expression %q", s.Cron)
		}
		*bits[i] = v
	}
	c.domRestricted, c.dowRestricted = fields[2] != "*", fields[4] != "*"
	return c, nil
}

// parse reads a field as ValidCron accepts it: a list of values, ranges,
// and steps, where "a/n" steps from a to the maximum.
func (r cronRange) parse(field string) (uint64, error) {
	var bits uint64
	for _, item := range strings.Split(field, ",") {
		base, step, hasStep := strings.Cut(item, "/")
		lo, hi := r.min, r.max
		if base != "*" {
			from, to, isRange := strings.Cut(base, "-")
			var err error
			if lo, err = integer(cronNumber(from, r)); err != nil {
				return 0, err
			}
			hi = lo
			if isRange {
				if hi, err = integer(cronNumber(to, r)); err != nil {
					return 0, err
				}
			} else if hasStep {
				hi = r.max
			}
		}
		n := 1
		if hasStep {
			var err error
			if n, err = integer(jsNumber(step)); err != nil {
				return 0, err
			}
		}
		for v := lo; v <= hi; v += n {
			bits |= 1 << v
		}
	}
	return bits, nil
}

// integer refuses a number that is not whole, which ValidCron accepts, such
// as "1.5".
func integer(n float64) (int, error) {
	if n != math.Trunc(n) {
		return 0, errors.Newf("%v is not a whole number", n)
	}
	return int(n), nil
}

// Next returns the first time after `after` that the schedule fires, and
// false when it never fires, such as on February 30. lastRun is when the
// schedule last fired, or the zero time: like GitHub, a schedule fires at
// most once every 5 minutes.
//
// In a time zone with daylight saving time, a time that the spring-forward
// transition skips fires at the end of the transition, as GitHub documents:
// 2:30 AM fires at 3:00 AM. A time that the fall-back transition repeats
// fires once, at its first occurrence.
func (c *Cron) Next(after, lastRun time.Time) (time.Time, bool) {
	if earliest := lastRun.Add(minScheduleInterval - time.Nanosecond); !lastRun.IsZero() && earliest.After(after) {
		after = earliest
	}
	local := after.In(c.location)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	start := local.Hour()*60 + local.Minute() + 1
	// Every day pattern repeats within 28 years; February 29 on a given
	// weekday is the rarest match.
	for i := 0; i < 366*28; i++ {
		if c.matchesDay(day) {
			for m := start; m < 24*60; m++ {
				if c.hour&(1<<(m/60)) == 0 {
					m += 59 - m%60
					continue
				}
				if c.minute&(1<<(m%60)) == 0 {
					continue
				}
				if t := c.instant(day, m/60, m%60); t.After(after) {
					return t, true
				}
			}
		}
		day, start = day.AddDate(0, 0, 1), 0
	}
	return time.Time{}, false
}

func (c *Cron) matchesDay(day time.Time) bool {
	if c.month&(1<<int(day.Month())) == 0 {
		return false
	}
	dom := c.dom&(1<<day.Day()) != 0
	dow := c.dow&(1<<int(day.Weekday())) != 0
	if c.domRestricted && c.dowRestricted {
		return dom || dow
	}
	return dom && dow
}

// instant returns the first instant at which the wall clock of the time
// zone reads the date and time, or the end of the transition that skips it.
func (c *Cron) instant(day time.Time, hour, minute int) time.Time {
	t := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, c.location)
	wall := func(t time.Time) bool {
		l := t.In(c.location)
		return l.Year() == day.Year() && l.YearDay() == day.YearDay() && l.Hour() == hour && l.Minute() == minute
	}
	if !wall(t) {
		// Go moves a skipped wall time to one side of the transition, which
		// is the nearer bound of the period t falls in.
		start, end := t.ZoneBounds()
		if !end.IsZero() && (start.IsZero() || end.Sub(t) < t.Sub(start)) {
			return end
		}
		return start
	}
	// A repeated wall time also reads the same in the period before.
	if start, _ := t.ZoneBounds(); !start.IsZero() {
		_, before := start.Add(-time.Nanosecond).Zone()
		_, now := t.Zone()
		if earlier := t.Add(time.Duration(now-before) * time.Second); earlier.Before(t) && wall(earlier) {
			return earlier
		}
	}
	return t
}
