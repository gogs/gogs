// Copyright 2013 gopm authors.
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

// Package log provides npm-like style log output.
package log

func Error(hl, msg string) {
	errorP(hl, msg)
}

func Fatal(hl, msg string) {
	fatal(hl, msg)
}

func Warn(format string, args ...interface{}) {
	warn(format, args...)
}

func Log(format string, args ...interface{}) {
	log(format, args...)
}

func Trace(format string, args ...interface{}) {
	trace(format, args...)
}

func Success(title, hl, msg string) {
	success(title, hl, msg)
}

func Message(hl, msg string) {
	message(hl, msg)
}

func Help(format string, args ...interface{}) {
	help(format, args...)
}
