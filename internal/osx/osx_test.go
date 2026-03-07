package osx

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{
			path: "osx.go",
			want: true,
		}, {
			path: "../osx",
			want: false,
		}, {
			path: "not_found",
			want: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, IsFile(test.path))
		})
	}
}

func TestIsDir(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{
			path: "osx.go",
			want: false,
		}, {
			path: "../osx",
			want: true,
		}, {
			path: "not_found",
			want: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, IsDir(test.path))
		})
	}
}

func TestExist(t *testing.T) {
	tests := []struct {
		path   string
		expVal bool
	}{
		{
			path:   "osx.go",
			expVal: true,
		}, {
			path:   "../osx",
			expVal: true,
		}, {
			path:   "not_found",
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, Exist(test.path))
		})
	}
}

func TestCurrentUsername(t *testing.T) {
	if oldUser, ok := os.LookupEnv("USER"); ok {
		defer func() { t.Setenv("USER", oldUser) }()
	} else {
		defer func() { _ = os.Unsetenv("USER") }()
	}

	t.Setenv("USER", "__TESTING::USERNAME")
	assert.Equal(t, "__TESTING::USERNAME", CurrentUsername())
}

func TestIsSymlink(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "symlink-test-*")
	require.NoError(t, err, "create temporary file")
	tempFilePath := tempFile.Name()
	_ = tempFile.Close()
	defer func() { _ = os.Remove(tempFilePath) }()

	// Create a temporary symlink
	tempSymlinkPath := tempFilePath + "-symlink"
	err = os.Symlink(tempFilePath, tempSymlinkPath)
	require.NoError(t, err, "create temporary symlink")
	defer func() { _ = os.Remove(tempSymlinkPath) }()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "non-existent path",
			path: "not_found",
			want: false,
		},
		{
			name: "regular file",
			path: tempFilePath,
			want: false,
		},
		{
			name: "symlink",
			path: tempSymlinkPath,
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, IsSymlink(test.path))
		})
	}
}
