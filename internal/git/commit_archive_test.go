package git

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func tempPath() string {
	return filepath.Join(os.TempDir(), strconv.Itoa(int(time.Now().UnixNano())))
}

func TestCommit_Archive(t *testing.T) {
	ctx := context.Background()
	for _, format := range []ArchiveFormat{
		ArchiveZip,
		ArchiveTarGz,
	} {
		t.Run(string(format), func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, "755fd577edcfd9209d0ac072eed3b022cbe4d39b")
			if err != nil {
				t.Fatal(err)
			}

			dst := tempPath()
			defer func() {
				_ = os.Remove(dst)
			}()

			assert.Nil(t, c.Archive(ctx, format, dst))
		})
	}
}
