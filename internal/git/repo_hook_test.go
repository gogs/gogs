package git

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepository_Hooks(t *testing.T) {
	t.Run("invalid hook", func(t *testing.T) {
		h, err := testrepo.Hook("", "bad_hook")
		assert.Equal(t, os.ErrNotExist, err)
		assert.Nil(t, h)
	})

	// Save "post-receive" hook with some content
	postReceiveHook := testrepo.NewHook(DefaultHooksDir, HookPostReceive)
	defer func() {
		_ = os.Remove(postReceiveHook.Path())
	}()

	err := postReceiveHook.Update("echo $1 $2 $3")
	if err != nil {
		t.Fatal(err)
	}

	hooks, err := testrepo.Hooks("")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 3, len(hooks))

	for i := range hooks {
		assert.NotEmpty(t, hooks[i].Content())
	}
}
