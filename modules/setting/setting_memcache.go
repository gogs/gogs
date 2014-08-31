// +build memcache

// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package setting

import (
	_ "github.com/macaron-contrib/cache/memcache"
)

func init() {
	EnableMemcache = true
}
