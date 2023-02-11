// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"fmt"
	"io"
)

// Logger is a wrapper of io.Writer for the GORM's logger.Writer.
type Logger struct {
	io.Writer
}

func (l *Logger) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(l.Writer, format, args...)
}
