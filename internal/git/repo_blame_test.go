package git

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepository_Blame(t *testing.T) {
	ctx := context.Background()

	t.Run("bad file", func(t *testing.T) {
		_, err := testrepo.Blame(ctx, "", "404.txt")
		assert.Error(t, err)
	})

	blame, err := testrepo.Blame(ctx, "cfc3b2993f74726356887a5ec093de50486dc617", "README.txt")
	assert.Nil(t, err)

	// Assert representative commits
	// https://github.com/gogs/git-module-testrepo/blame/master/README.txt
	tests := []struct {
		line  int
		expID string
	}{
		{line: 1, expID: "755fd577edcfd9209d0ac072eed3b022cbe4d39b"},
		{line: 3, expID: "a13dba1e469944772490909daa58c53ac8fa4b0d"},
		{line: 5, expID: "755fd577edcfd9209d0ac072eed3b022cbe4d39b"},
		{line: 13, expID: "8d2636da55da593c421e1cb09eea502a05556a69"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("Line %d", test.line), func(t *testing.T) {
			line := blame.Line(test.line)
			assert.Equal(t, test.expID, line.ID.String())
		})
	}
}
