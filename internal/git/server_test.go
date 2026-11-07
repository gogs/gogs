package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateServerInfo(t *testing.T) {
	ctx := context.Background()
	err := os.RemoveAll(filepath.Join(repoPath, "info"))
	require.NoError(t, err)
	err = UpdateServerInfo(ctx, repoPath, UpdateServerInfoOptions{Force: true})
	require.NoError(t, err)
	assert.True(t, isFile(filepath.Join(repoPath, "info", "refs")))
}

func TestReceivePack(t *testing.T) {
	ctx := context.Background()
	got, err := ReceivePack(ctx, repoPath, ReceivePackOptions{HTTPBackendInfoRefs: true})
	require.NoError(t, err)
	const contains = "report-status report-status-v2 delete-refs side-band-64k quiet atomic ofs-delta object-format=sha1 agent=git/"
	assert.Contains(t, string(got), contains)
}

func TestUploadPack(t *testing.T) {
	ctx := context.Background()
	got, err := UploadPack(ctx, repoPath,
		UploadPackOptions{
			StatelessRPC:        true,
			Strict:              true,
			HTTPBackendInfoRefs: true,
		},
	)
	require.NoError(t, err)
	const contains = "multi_ack thin-pack side-band side-band-64k ofs-delta shallow deepen-since deepen-not deepen-relative no-progress include-tag multi_ack_detailed no-done symref=HEAD:refs/heads/master object-format=sha1 agent=git/"
	assert.Contains(t, string(got), contains)
}
