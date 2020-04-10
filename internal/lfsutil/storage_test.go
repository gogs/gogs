// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	s := &LocalStorage{
		Root: filepath.Join(os.TempDir(), "lfs-objects"),
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(s.Root)
	})

	tests := []struct {
		name       string
		oid        OID
		content    string
		expWritten int64
		expErr     error
	}{
		{
			name:   "invalid oid",
			oid:    "bad_oid",
			expErr: ErrInvalidOID,
		},

		{
			name:       "valid oid",
			oid:        "ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
			content:    "Hello world!",
			expWritten: 12,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			written, err := s.Upload(test.oid, ioutil.NopCloser(strings.NewReader(test.content)))
			assert.Equal(t, test.expWritten, written)
			assert.Equal(t, test.expErr, err)
		})
	}
}

func TestLocalStorage_Download(t *testing.T) {
	oid := OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f")
	s := &LocalStorage{
		Root: filepath.Join(os.TempDir(), "lfs-objects"),
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(s.Root)
	})

	fpath := s.storagePath(oid)
	err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(fpath, []byte("Hello world!"), os.ModePerm)
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
