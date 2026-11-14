package dbx

import (
	"fmt"
	"strings"

	"unknwon.dev/x/logx"
)

// Logger adapts a logx.Logger to GORM's logger.Writer. Lines that GORM marks
// as errors are logged at error level. Everything else is debug, which covers
// SQL in non-production and stays quiet once the process level is info.
type Logger struct {
	logger *logx.Logger
}

// NewLogger returns a GORM writer that logs through the given logger.
func NewLogger(logger *logx.Logger) Logger {
	return Logger{logger: logger}
}

func (l Logger) Printf(format string, args ...any) {
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	if strings.Contains(msg, "[error]") {
		l.logger.Error(msg)
		return
	}
	l.logger.Debug(msg)
}
