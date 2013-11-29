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

// +build !windows

package log

import (
	"fmt"
	"os"

	"github.com/aybabtme/color/brush"
)

func Error(hl, msg string) {
	if len(hl) > 0 {
		hl = " " + brush.Red(hl).String()
	}
	fmt.Printf("gopm %s%s %s\n", brush.Red("ERR!"), hl, msg)
}

func Fatal(hl, msg string) {
	Error(hl, msg)
	os.Exit(2)
}

func Log(format string, args ...interface{}) {
	fmt.Printf("gopm %s %s\n", brush.White("INFO"),
		fmt.Sprintf(format, args...))
}

func Trace(format string, args ...interface{}) {
	fmt.Printf("gopm %s %s\n", brush.Blue("TRAC"),
		fmt.Sprintf(format, args...))
}

func Success(title, hl, msg string) {
	if len(hl) > 0 {
		hl = " " + brush.Green(hl).String()
	}
	fmt.Printf("gopm %s%s %s\n", brush.Green(title), hl, msg)
}

func Message(hl, msg string) {
	if len(hl) > 0 {
		hl = " " + brush.Yellow(hl).String()
	}
	fmt.Printf("gopm %s%s %s\n", brush.Yellow("MSG!"), hl, msg)
}
