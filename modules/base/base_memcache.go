// +build memcache

package base

import (
	_ "github.com/gogits/cache/memcache"
)

func init() {
	EnableMemcache = true
}
