// +build redis

package base

import (
	_ "github.com/gogits/cache/redis"
)

func init() {
	EnableRedis = true
}
