package markup_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "gogs.io/gogs/internal/markup"
)

func TestRawOrgMode_Include(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("SECRET_KEY = s3cr3t"), 0o600))

	for _, kind := range []string{"src text", "example text", "export html"} {
		t.Run(kind, func(t *testing.T) {
			body := fmt.Sprintf("* Heading\n#+INCLUDE: %q %s\n", secret, kind)
			got := string(RawOrgMode([]byte(body), ""))
			assert.Contains(t, got, "Heading")
			assert.NotContains(t, got, "s3cr3t")
		})
	}
}
