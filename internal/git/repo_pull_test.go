package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_MergeBase(t *testing.T) {
	ctx := context.Background()

	t.Run("bad revision", func(t *testing.T) {
		// "bad_revision" doesn't exist, so git fails with exit status 128 (fatal),
		// not exit status 1 (no merge base).
		mb, err := testrepo.MergeBase(ctx, "0eedd79eba4394bbef888c804e899731644367fe", "bad_revision")
		assert.Error(t, err)
		assert.Empty(t, mb)
	})

	tests := []struct {
		base          string
		head          string
		opt           MergeBaseOptions
		wantMergeBase string
	}{
		{
			base:          "4e59b72440188e7c2578299fc28ea425fbe9aece",
			head:          "0eedd79eba4394bbef888c804e899731644367fe",
			wantMergeBase: "4e59b72440188e7c2578299fc28ea425fbe9aece",
		},
		{
			base:          "master",
			head:          "release-1.0",
			wantMergeBase: "0eedd79eba4394bbef888c804e899731644367fe",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			mb, err := testrepo.MergeBase(ctx, test.base, test.head, test.opt)
			require.NoError(t, err)
			assert.Equal(t, test.wantMergeBase, mb)
		})
	}
}
