package git

import (
	"bytes"
	"context"
	"flag"
	stdlog "log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const repoPath = "testdata/testrepo.git"

var testrepo *Repository

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Verbose() {
		SetOutput(os.Stdout)
	}

	ctx := context.Background()

	// Set up the test repository
	if !isExist(repoPath) {
		if err := Clone(ctx, "https://github.com/gogs/git-module-testrepo.git", repoPath, CloneOptions{
			Bare: true,
		}); err != nil {
			stdlog.Fatal(err)
		}
	}

	var err error
	testrepo, err = Open(repoPath)
	if err != nil {
		stdlog.Fatal(err)
	}

	os.Exit(m.Run())
}

func TestSetPrefix(t *testing.T) {
	old := logPrefix
	new := "[custom] "
	SetPrefix(new)
	defer SetPrefix(old)

	assert.Equal(t, new, logPrefix)
}

func Test_log(t *testing.T) {
	old := logOutput
	defer SetOutput(old)

	tests := []struct {
		format    string
		args      []interface{}
		expOutput string
	}{
		{
			format:    "",
			expOutput: "[git-module] \n",
		},
		{
			format:    "something",
			expOutput: "[git-module] something\n",
		},
		{
			format:    "val: %v",
			args:      []interface{}{123},
			expOutput: "[git-module] val: 123\n",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			var buf bytes.Buffer
			SetOutput(&buf)

			logf(test.format, test.args...)
			assert.Equal(t, test.expOutput, buf.String())
		})
	}
}
