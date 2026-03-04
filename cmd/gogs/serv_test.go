package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSSHCmd(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantVerb string
		wantArgs string
	}{
		{
			name:     "git-upload-pack",
			cmd:      "git-upload-pack '/owner/repo.git'",
			wantVerb: "git-upload-pack",
			wantArgs: "'owner/repo.git'",
		},
		{
			name:     "git-lfs-transfer upload",
			cmd:      "git-lfs-transfer '/owner/repo.git' upload",
			wantVerb: "git-lfs-transfer",
			wantArgs: "'owner/repo.git' upload",
		},
		{
			name: "empty command",
			cmd:  "git-upload-pack",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verb, args := parseSSHCmd(test.cmd)
			assert.Equal(t, test.wantVerb, verb)
			assert.Equal(t, test.wantArgs, args)
		})
	}
}

func TestParseLFSTransferArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          string
		wantRepoPath  string
		wantOperation string
		wantOk        bool
	}{
		{
			name:          "quoted path with upload",
			args:          "'owner/repo.git' upload",
			wantRepoPath:  "owner/repo.git",
			wantOperation: "upload",
			wantOk:        true,
		},
		{
			name:          "quoted path with download",
			args:          "'owner/repo.git' download",
			wantRepoPath:  "owner/repo.git",
			wantOperation: "download",
			wantOk:        true,
		},
		{
			name:          "unquoted path",
			args:          "owner/repo.git upload",
			wantRepoPath:  "owner/repo.git",
			wantOperation: "upload",
			wantOk:        true,
		},
		{
			name:          "path with leading slash",
			args:          "/owner/repo.git upload",
			wantRepoPath:  "owner/repo.git",
			wantOperation: "upload",
			wantOk:        true,
		},
		{
			name:          "quoted path with leading slash",
			args:          "'/owner/repo.git' download",
			wantRepoPath:  "owner/repo.git",
			wantOperation: "download",
			wantOk:        true,
		},
		{
			name: "invalid operation",
			args: "'owner/repo.git' push",
		},
		{
			name: "no space separator",
			args: "owner/repo.git",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repoPath, operation, ok := parseLFSTransferArgs(test.args)
			assert.Equal(t, test.wantOk, ok)
			if ok {
				assert.Equal(t, test.wantRepoPath, repoPath)
				assert.Equal(t, test.wantOperation, operation)
			}
		})
	}
}
