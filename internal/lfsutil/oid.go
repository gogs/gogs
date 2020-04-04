// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

import (
	"strings"
)

// ValidateOID returns true if given oid is validate according to spec:
// https://github.com/git-lfs/git-lfs/blob/master/docs/spec.md
func ValidateOID(oid string) bool {
	fields := strings.SplitN(oid, ":", 2)
	if len(fields) != 2 {
		return false
	}
	method := fields[0]
	hash := fields[1]

	switch method {
	case "sha256":
		// SHA256 produces 64-char lower case hexadecimal hash
		return len(hash) == 64 && strings.ToLower(hash) == hash
	}
	return false
}
