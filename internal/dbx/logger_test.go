package dbx

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"unknwon.dev/x/logx"
)

func TestLogger_Trace(t *testing.T) {
	query := func() (string, int64) { return "SELECT * FROM `user`", 1 }
	tests := []struct {
		name      string
		level     gormlogger.LogLevel
		elapsed   time.Duration
		err       error
		wantLevel string
		wantMsg   string
	}{
		{name: "query", level: gormlogger.Info, wantLevel: "DEBUG", wantMsg: "Query"},
		{name: "record not found is not an error", level: gormlogger.Info, err: gormlogger.ErrRecordNotFound, wantLevel: "DEBUG", wantMsg: "Query"},
		{name: "failed query", level: gormlogger.Warn, err: errors.New("no such table"), wantLevel: "ERROR", wantMsg: "Query failed"},
		{name: "slow query", level: gormlogger.Warn, elapsed: time.Second, wantLevel: "WARN", wantMsg: "Slow query"},
		{name: "fast query below info level", level: gormlogger.Warn},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logx.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			l := NewLogger(logger, test.level, 100*time.Millisecond)

			l.Trace(context.Background(), time.Now().Add(-test.elapsed), query, test.err)

			out := strings.TrimSpace(buf.String())
			if test.wantMsg == "" {
				assert.Empty(t, out)
				return
			}
			assert.NotContains(t, out, "\n")
			assert.Contains(t, out, `"level":"`+test.wantLevel+`"`)
			assert.Contains(t, out, `"msg":"`+test.wantMsg+`"`)
			assert.Contains(t, out, "\"sql\":\"SELECT * FROM `user`\"")
			assert.Contains(t, out, `"rows":1`)
		})
	}
}

func TestLogger_ParamsFilter(t *testing.T) {
	tests := []struct {
		name       string
		level      gormlogger.LogLevel
		wantValues bool
	}{
		{name: "values are hidden when only failures are logged", level: gormlogger.Warn},
		{name: "values are only on the debug line when every query is logged", level: gormlogger.Info, wantValues: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logx.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: NewLogger(logger, test.level, 0)})
			require.NoError(t, err)
			require.NoError(t, db.Exec("CREATE TABLE secret (name TEXT UNIQUE, passwd TEXT)").Error)
			require.NoError(t, db.Exec("INSERT INTO secret VALUES (?, ?)", "alice", "hashed-password").Error)
			buf.Reset()

			err = db.Exec("INSERT INTO secret VALUES (?, ?)", "alice", "hashed-password").Error
			require.Error(t, err)

			var failedLine, debugLines string
			for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
				if strings.Contains(line, `"level":"ERROR"`) {
					failedLine = line
				} else {
					debugLines += line
				}
			}
			assert.Contains(t, failedLine, `"msg":"Query failed"`)
			// Error lines may reach Slack or Discord, so they never carry values.
			assert.NotContains(t, failedLine, "hashed-password")
			if test.wantValues {
				assert.Contains(t, debugLines, "hashed-password")
			} else {
				assert.Empty(t, debugLines)
				assert.Contains(t, failedLine, "VALUES (?, ?)")
			}
		})
	}
}
