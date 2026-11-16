package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCronNext(t *testing.T) {
	utc := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04", s)
		require.NoError(t, err)
		return v
	}
	tests := []struct {
		name, cron, timezone string
		after, lastRun       string
		want                 string // Empty when the schedule never fires.
	}{
		{"weekdays skip the weekend", "30 5 * * 1-5", "", "2026-11-13 05:30", "", "2026-11-16 05:30"},
		{"names", "0 0 * JAN sun", "", "2026-11-16 00:00", "", "2027-01-03 00:00"},
		{"a step starts at its base", "20/15 * * * *", "", "2026-11-16 10:50", "", "2026-11-16 11:20"},
		// POSIX matches either day field when both are restricted, as GitHub
		// confirmed in github.com/orgs/community/discussions/12804: October
		// 7 2026 is a Wednesday, and the next Saturday is the 10th.
		{"day of month or day of week", "0 9 1-7 * 6", "", "2026-10-07 09:00", "", "2026-10-10 09:00"},
		{"day of month alone", "0 9 1-7 * *", "", "2026-10-07 09:00", "", "2026-11-01 09:00"},
		{"time zone", "30 5 * * 1-5", "America/New_York", "2026-11-16 00:00", "", "2026-11-16 10:30"},
		// March 14 2027 skips 2:00 to 3:00 in New York: 2:30 runs at 3:00 EDT.
		{"spring forward", "30 2 * * *", "America/New_York", "2027-03-13 12:00", "", "2027-03-14 07:00"},
		// November 1 2026 repeats 1:00 to 2:00: 1:30 runs once, in EDT.
		{"fall back", "30 1 * * *", "America/New_York", "2026-10-31 12:00", "", "2026-11-01 05:30"},
		{"fall back, next day", "30 1 * * *", "America/New_York", "2026-11-01 05:30", "", "2026-11-02 06:30"},
		{"at most every 5 minutes", "* * * * *", "", "2026-11-16 10:01", "2026-11-16 10:00", "2026-11-16 10:05"},
		{"February 30", "0 0 30 2 *", "", "2026-11-16 00:00", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseSchedule(Schedule{Cron: tt.cron, Timezone: tt.timezone})
			require.NoError(t, err)
			var lastRun time.Time
			if tt.lastRun != "" {
				lastRun = utc(tt.lastRun)
			}
			got, ok := c.Next(utc(tt.after), lastRun)
			if tt.want == "" {
				assert.False(t, ok, got)
				return
			}
			require.True(t, ok)
			assert.Equal(t, utc(tt.want), got.UTC())
		})
	}

	for _, s := range []Schedule{{Cron: "0 0 * * 7"}, {Cron: "@daily"}, {Cron: "0 0 * * *", Timezone: "Mars/Olympus"}, {Cron: "0 0 * * *", Timezone: "Local"}} {
		_, err := ParseSchedule(s)
		assert.Error(t, err, s)
	}
}
