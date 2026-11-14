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

func (l *Logger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	// FileWithLineNum skips its caller's frame, so it must be called here and
	// not inside the closure below.
	source := utils.FileWithLineNum()
	attrs := func() []any {
		sql, rows := fc()
		attrs := []any{"sql", sql, "rows", rows, "elapsed", elapsed.String(), "source", source}
		if err != nil {
			attrs = append(attrs, "error", err)
		}
		return attrs
	}

	notFound := errors.Is(err, gormlogger.ErrRecordNotFound)
	switch {
	case err != nil && !notFound && l.level >= gormlogger.Error:
		l.logger.Error("Query failed", attrs()...)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		l.logger.Warn("Slow query", append(attrs(), "threshold", l.slowThreshold.String())...)
	case l.level >= gormlogger.Info:
		l.logger.Debug("Query", attrs()...)
	}
}
