// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package log is a wrapper of logs for short calling name.
package log

import (
	"github.com/gogits/logs"
)

var (
	logger       *logs.BeeLogger
	Mode, Config string
)

func init() {
	NewLogger(0, "console", `{"level": 0}`)
}

func NewLogger(bufLen int64, mode, config string) {
	Mode, Config = mode, config
	logger = logs.NewLogger(bufLen)
	logger.SetLogFuncCallDepth(3)
	logger.SetLogger(mode, config)
}

func Trace(format string, v ...interface{}) {
	logger.Trace(format, v...)
}

func Debug(format string, v ...interface{}) {
	logger.Debug(format, v...)
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

func Critical(format string, v ...interface{}) {
	logger.Critical(format, v...)
}
