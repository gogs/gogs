package lfs

import (
	"flag"
	"os"
	"testing"

	"gogs.io/gogs/internal/applog"
	"gogs.io/gogs/internal/logx"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if !testing.Verbose() {
		applog.Logger = logx.NewNoop()
	}
	os.Exit(m.Run())
}
