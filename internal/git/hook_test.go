package git

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHook(t *testing.T) {
	path := tempPath()
	h := &Hook{
		name:     HookPreReceive,
		path:     path,
		isSample: false,
		content:  "test content",
	}

	assert.Equal(t, HookPreReceive, h.Name())
	assert.Equal(t, path, h.Path())
	assert.False(t, h.IsSample())
	assert.Equal(t, "test content", h.Content())
}

func TestHook_Update(t *testing.T) {
	path := tempPath()
	defer func() {
		_ = os.Remove(path)
	}()

	h := &Hook{
		name:     HookPreReceive,
		path:     path,
		isSample: false,
	}
	err := h.Update("test content")
	if err != nil {
		t.Fatal(err)
	}

	p, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "test content", string(p))

	t.Run("empty content removes the hook", func(t *testing.T) {
		err := h.Update(" \r\n ")
		require.NoError(t, err)

		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err))
		assert.True(t, h.IsSample())
		assert.Equal(t, serverSideHookSamples[HookPreReceive], h.Content())

		// Removing a hook that does not exist is not an error.
		require.NoError(t, h.Update(""))
	})
}
