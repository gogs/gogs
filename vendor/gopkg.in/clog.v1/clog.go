// Copyright 2017 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// Clog is a channel-based logging package for Go.
package clog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	_VERSION = "1.1.0"
)

// Version returns current version of the package.
func Version() string {
	return _VERSION
}

type (
	MODE  string
	LEVEL int
)

const (
	TRACE LEVEL = iota
	INFO
	WARN
	ERROR
	FATAL
)

var formats = map[LEVEL]string{
	TRACE: "[TRACE] ",
	INFO:  "[ INFO] ",
	WARN:  "[ WARN] ",
	ERROR: "[ERROR] ",
	FATAL: "[FATAL] ",
}

// isValidLevel returns true if given level is in the valid range.
func isValidLevel(level LEVEL) bool {
	return level >= TRACE && level <= FATAL
}

// Message represents a log message to be processed.
type Message struct {
	Level LEVEL
	Body  string
}

func Write(level LEVEL, skip int, format string, v ...interface{}) {
	msg := &Message{
		Level: level,
	}

	// Only error and fatal information needs locate position for debugging.
	// But if skip is 0 means caller doesn't care so we can skip.
	if msg.Level >= ERROR && skip > 0 {
		pc, file, line, ok := runtime.Caller(skip)
		if ok {
			// Get caller function name.
			fn := runtime.FuncForPC(pc)
			var fnName string
			if fn == nil {
				fnName = "?()"
			} else {
				fnName = strings.TrimLeft(filepath.Ext(fn.Name()), ".") + "()"
			}

			if len(file) > 20 {
				file = "..." + file[len(file)-20:]
			}
			msg.Body = formats[level] + fmt.Sprintf("[%s:%d %s] ", file, line, fnName) + fmt.Sprintf(format, v...)
		}
	}
	if len(msg.Body) == 0 {
		msg.Body = formats[level] + fmt.Sprintf(format, v...)
	}

	for i := range receivers {
		if receivers[i].Level() > level {
			continue
		}

		receivers[i].msgChan <- msg
	}
}

func Trace(format string, v ...interface{}) {
	Write(TRACE, 0, format, v...)
}

func Info(format string, v ...interface{}) {
	Write(INFO, 0, format, v...)
}

func Warn(format string, v ...interface{}) {
	Write(WARN, 0, format, v...)
}

func Error(skip int, format string, v ...interface{}) {
	Write(ERROR, skip, format, v...)
}

func Fatal(skip int, format string, v ...interface{}) {
	Write(FATAL, skip, format, v...)
	Shutdown()
	os.Exit(1)
}

func Shutdown() {
	for i := range receivers {
		receivers[i].Destroy()
	}

	// Shutdown the error handling goroutine.
	quitChan <- struct{}{}
	for {
		if len(errorChan) == 0 {
			break
		}

		fmt.Printf("clog: unable to write message: %v\n", <-errorChan)
	}
}
