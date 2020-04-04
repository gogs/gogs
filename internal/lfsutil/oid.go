// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

import (
	"gogs.io/gogs/internal/lazyregexp"
)

// OID is an LFS object ID.
type OID string

// An OID is a 64-char lower case hexadecimal, produced by SHA256.
// Spec: https://github.com/git-lfs/git-lfs/blob/master/docs/spec.md
var oidRe = lazyregexp.New("^[a-f0-9]{64}$")

// ValidOID returns true if given oid is valid.
func ValidOID(oid OID) bool {
	return oidRe.MatchString(string(oid))
}
