package lfsutil

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/osutil"
)

func TestLocalStorage_storagePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	s := &LocalStorage{
		Root: "/lfs-objects",
	}

	tests := []struct {
		name    string
		oid     OID
		expPath string
	}{
		{
			name: "empty oid",
			oid:  "",
		},

		{
			name:    "valid oid",
			oid:     "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
			expPath: "/lfs-objects/e/f/ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expPath, s.storagePath(test.oid))
		})
	}
}

func TestLocalStorage_Upload(t *testing.T) {
	base := t.TempDir()
	s := &LocalStorage{
		Root:    filepath.Join(base, "lfs-objects"),
		TempDir: filepath.Join(base, "tmp", "lfs"),
	}

	const helloWorldOID = OID("c0535e4be2b79ffd93291305436bf889314e4a3faec05ecffcbb7df31ad9e51a") // "Hello world!"

	t.Run("invalid OID", func(t *testing.T) {
		written, err := s.Upload("bad_oid", io.NopCloser(strings.NewReader("")))
		assert.Equal(t, int64(0), written)
		assert.Equal(t, ErrInvalidOID, err)
	})

	t.Run("valid OID", func(t *testing.T) {
		written, err := s.Upload(helloWorldOID, io.NopCloser(strings.NewReader("Hello world!")))
		require.NoError(t, err)
		assert.Equal(t, int64(12), written)
	})

	t.Run("valid OID but wrong content", func(t *testing.T) {
		oid := OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
		written, err := s.Upload(oid, io.NopCloser(strings.NewReader("Hello world!")))
		assert.Equal(t, int64(0), written)
		assert.Equal(t, ErrOIDMismatch, err)

		// File should have been cleaned up.
		assert.False(t, osutil.IsFile(s.storagePath(oid)))
	})

	t.Run("duplicate upload returns existing size", func(t *testing.T) {
		written, err := s.Upload(helloWorldOID, io.NopCloser(strings.NewReader("should be ignored")))
		require.NoError(t, err)
		assert.Equal(t, int64(12), written)

		// Verify original content is preserved.
		var buf bytes.Buffer
		err = s.Download(helloWorldOID, &buf)
		require.NoError(t, err)
		assert.Equal(t, "Hello world!", buf.String())
	})
}

func TestLocalStorage_Download(t *testing.T) {
	oid := OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	s := &LocalStorage{
		Root: filepath.Join(t.TempDir(), "lfs-objects"),
	}

	fpath := s.storagePath(oid)
	err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(fpath, []byte("Hello world!"), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		oid        OID
		expContent string
		expErr     error
	}{
		{
			name:   "object not exists",
			oid:    "bad_oid",
			expErr: ErrObjectNotExist,
		},

		{
			name:       "valid oid",
			oid:        oid,
			expContent: "Hello world!",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := s.Download(test.oid, &buf)
			assert.Equal(t, test.expContent, buf.String())
			assert.Equal(t, test.expErr, err)
		})
	}
}
