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

package log

import (
	"fmt"
	"os"
)

func Error(hl, msg string) {
	if len(hl) > 0 {
		hl = " " + hl
	}
	fmt.Printf("gopm ERR!%s %s\n", hl, msg)
}

func Fatal(hl, msg string) {
	Error(hl, msg)
	os.Exit(2)
}

func Log(format string, args ...interface{}) {
	fmt.Printf("gopm INFO %s\n",
		fmt.Sprintf(format, args...))
}

func Trace(format string, args ...interface{}) {
	fmt.Printf("gopm TRAC %s\n",
		fmt.Sprintf(format, args...))
}

func Success(title, hl, msg string) {
	if len(hl) > 0 {
		hl = " " + hl
	}
	fmt.Printf("gopm %s%s %s\n", title, hl, msg)
}

func Message(hl, msg string) {
	if len(hl) > 0 {
		hl = " " + hl
	}
	fmt.Printf("gopm MSG!%s %s\n", hl, msg)
}
