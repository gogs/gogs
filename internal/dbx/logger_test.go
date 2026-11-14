package dbx

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
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
