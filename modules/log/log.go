// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package log is a wrapper of logs for short calling name.
package log

import (
	"github.com/gogits/logs"
)

var (
	loggers []*logs.BeeLogger
)

func init() {
	NewLogger(0, "console", `{"level": 0}`)
}

func NewLogger(bufLen int64, mode, config string) {
	logger := logs.NewLogger(bufLen)

	isExist := false
	for _, l := range loggers {
		if l.Adapter == mode {
			isExist = true
			l = logger
		}
	}
	if !isExist {
		loggers = append(loggers, logger)
	}
	logger.SetLogFuncCallDepth(3)
	logger.SetLogger(mode, config)
}

func Trace(format string, v ...interface{}) {
	for _, logger := range loggers {
		logger.Trace(format, v...)
	}
}

func Debug(format string, v ...interface{}) {
	for _, logger := range loggers {
		logger.Debug(format, v...)
	}
}

func Info(format string, v ...interface{}) {
	for _, logger := range loggers {
		logger.Info(format, v...)
	}
}

func Error(format string, v ...interface{}) {
	for _, logger := range loggers {
		logger.Error(format, v...)
	}
}

func Warn(format string, v ...interface{}) {
	for _, logger := range loggers {
		logger.Warn(format, v...)
	}
}

func Critical(format string, v ...interface{}) {
	for _, logger := range loggers {
		logger.Critical(format, v...)
	}
}
