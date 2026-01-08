package lfs

import (
	"flag"
	"fmt"
	"os"
	"testing"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/testutil"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if !testing.Verbose() {
		// Remove the primary logger and register a noop logger.
		log.Remove(log.DefaultConsoleName)
		err := log.New("noop", testutil.InitNoopLogger)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
