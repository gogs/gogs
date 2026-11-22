package ssh

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpanName(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "fetch", command: "git-upload-pack 'alice/example.git'", want: "ssh git-upload-pack"},
		{name: "push", command: "git-receive-pack '/alice/example.git'", want: "ssh git-receive-pack"},
		{name: "archive", command: "git-upload-archive 'alice/example.git'", want: "ssh git-upload-archive"},
		{name: "unknown command", command: "git-upload-pack-" + strings.Repeat("x", 4096) + " 'alice/example.git'", want: "ssh"},
		{name: "known command without a repository", command: "git-upload-pack", want: "ssh"},
		{name: "empty", command: "", want: "ssh"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, spanName(test.command))
		})
	}
}
