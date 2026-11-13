package dbx

import (
	"fmt"
	"strings"

	"gogs.io/gogs/internal/applog"
)

// Logger adapts the process logger to GORM's logger.Writer. Lines that GORM
// marks as errors are logged at error level. Everything else is debug, which
// covers SQL in non-production and stays quiet once the process level is info.
type Logger struct{}

func (Logger) Printf(format string, args ...any) {
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	print := applog.Logger.Scoped("gorm").Debugf
	if strings.Contains(msg, "[error]") {
		print = applog.Logger.Scoped("gorm").Errorf
	}
	print("%s", msg)
}
