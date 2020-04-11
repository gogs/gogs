// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"fmt"
	"io"
)

// Writer is a wrapper of io.Writer for the gorm.logger.
type Writer struct {
	io.Writer
}

func (w *Writer) Print(v ...interface{}) {
	if len(v) == 0 {
		return
	}

	if len(v) == 1 {
		fmt.Fprint(w.Writer, v[0])
		return
	}

	switch v[0] {
	case "sql":
		fmt.Fprintf(w.Writer, "[sql] [%s] [%s] %s %v (%d rows affected)", v[1:]...)
	case "log":
		fmt.Fprintf(w.Writer, "[log] [%s] %s", v[1:]...)
	case "error":
		fmt.Fprintf(w.Writer, "[err] [%s] %s", v[1:]...)
	default:
		fmt.Fprint(w.Writer, v...)
	}
}
