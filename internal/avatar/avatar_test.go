package avatar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RandomImage(t *testing.T) {
	_, err := RandomImage([]byte("gogs@local"))
	assert.NoError(t, err)
	_, err = RandomImageWithSize(0, []byte("gogs@local"))
	assert.Error(t, err)
}
