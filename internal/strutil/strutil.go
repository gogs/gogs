// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package strutil

import (
	"unicode"
)

// ToUpperFirst returns s with only the first Unicode letter mapped to its upper case.
func ToUpperFirst(s string) string {
	for i, v := range s {
		return string(unicode.ToUpper(v)) + s[i+1:]
	}
	return ""
}
