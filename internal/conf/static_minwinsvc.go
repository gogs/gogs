//go:build minwinsvc

package conf

import (
	_ "github.com/gogs/minwinsvc"
)

func init() {
	HasMinWinSvc = true
}
