package dbx

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
	"unknwon.dev/x/logx"
)

// Logger is a GORM logger that writes one structured line per query. Failed
// queries are errors, slow queries are warnings, and every other query is
// debug.
type Logger struct {
	logger        *logx.Logger
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

// NewLogger returns a GORM logger that logs through the given logger.
func NewLogger(logger *logx.Logger, level gormlogger.LogLevel, slowThreshold time.Duration) *Logger {
	return &Logger{
		logger:        logger,
		level:         level,
		slowThreshold: slowThreshold,
	}
}

func (l *Logger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *Logger) Info(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Info {
		l.logger.Debug(fmt.Sprintf(msg, args...), "source", utils.FileWithLineNum())
	}
}

func (l *Logger) Warn(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Warn {
		l.logger.Warn(fmt.Sprintf(msg, args...), "source", utils.FileWithLineNum())
	}
}

func (l *Logger) Error(_ context.Context, msg string, args ...any) {
	if l.level >= gormlogger.Error {
		l.logger.Error(fmt.Sprintf(msg, args...), "source", utils.FileWithLineNum())
	}
}

// ParamsFilter implements gorm.ParamsFilter. Query values are only logged when
// every query is logged. Otherwise only failed and slow queries are, which may
// go to Slack or Discord and include password hashes or tokens.
func (l *Logger) ParamsFilter(_ context.Context, sql string, params ...any) (string, []any) {
	if l.level >= gormlogger.Info {
		return sql, params
	}
	return sql, nil
}

func (l *Logger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	notFound := errors.Is(err, gormlogger.ErrRecordNotFound)
	failed := err != nil && !notFound && l.level >= gormlogger.Error
	slow := !failed && l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn
	if !failed && !slow && l.level < gormlogger.Info {
		return
	}

	sql, rows := fc()
	attrs := []any{"sql", sql, "rows", rows, "elapsed", elapsed.String(), "source", utils.FileWithLineNum()}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	if l.level >= gormlogger.Info {
		// The SQL includes its values (see ParamsFilter), so it stays on this debug
		// line and off the error and warning that Slack or Discord may receive.
		l.logger.Debug("Query", attrs...)
		attrs = attrs[2:]
	}
	switch {
	case failed:
		l.logger.Error("Query failed", attrs...)
	case slow:
		l.logger.Warn("Slow query", append(attrs, "threshold", l.slowThreshold.String())...)
	}
}
