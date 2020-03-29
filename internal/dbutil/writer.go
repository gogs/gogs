// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"fmt"
	"io"

	"github.com/jinzhu/gorm/logger"
)

var _ logger.Writer = (*Writer)(nil)

// Writer is a wrapper of io.Writer for the logger.Interface.
type Writer struct {
	io.Writer
}

func (w *Writer) Printf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(w.Writer, format, args...)
}
