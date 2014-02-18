// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package log is a wrapper of logs for short calling name.
package log

import (
	"github.com/gogits/logs"
)

var logger *logs.BeeLogger

func init() {
	logger = logs.NewLogger(10000)
	logger.SetLogger("console", "")
}

func Info(format string, v ...interface{}) {
	logger.Info(format, v...)
}

func Error(format string, v ...interface{}) {
	logger.Error(format, v...)
}

func Warn(format string, v ...interface{}) {
	logger.Warn(format, v...)
}
