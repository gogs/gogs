package embeddedpg

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitialize(t *testing.T) {
	workDir := "/tmp/gogs-test"
	pg := Initialize(workDir)

	assert.NotNil(t, pg)
	assert.Equal(t, filepath.Join(workDir, "data", "local-postgres"), pg.baseDir)
	assert.Equal(t, uint32(15432), pg.tcpPort)
	assert.Equal(t, "gogs", pg.database)
	assert.Equal(t, "gogs", pg.user)
	assert.Equal(t, "gogs", pg.pass)
}

func TestShutdownWithoutStart(t *testing.T) {
	pg := Initialize("/tmp/gogs-test")

	// Should not error when stopping a non-started instance
	err := pg.Shutdown()
	assert.NoError(t, err)
}
