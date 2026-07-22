package repo

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
)

func TestIsGitNotValidObjectNameError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			want: false,
		},
		{
			name: "current Git error",
			err:  errors.New("fatal: Not a valid object name feature-branch"),
			want: true,
		},
		{
			name: "lowercase Git error",
			err:  errors.New("fatal: not a valid object name feature-branch"),
			want: true,
		},
		{
			name: "unrelated Git error",
			err:  errors.New("fatal: ambiguous argument feature-branch"),
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, isGitNotValidObjectNameError(test.err))
		})
	}
}
