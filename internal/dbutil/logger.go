package dbutil

import (
	"fmt"
	"io"
)

// Logger is a wrapper of io.Writer for the GORM's logger.Writer.
type Logger struct {
	io.Writer
}

func (l *Logger) Printf(format string, args ...interface{}) {
	fmt.Fprintf(l.Writer, format, args...)
}
