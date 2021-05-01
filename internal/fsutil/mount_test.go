// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fsutil_test

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	. "gogs.io/gogs/internal/fsutil"
)

var testFsys = fstest.MapFS{
	"hello.txt": {
		Data:    []byte("hello, world"),
		Mode:    0456,
		ModTime: time.Now(),
		Sys:     &sysValue,
	},
	"goodbye.txt": {
		Data:    []byte("goodbye, world"),
		Mode:    0456,
		ModTime: time.Now(),
		Sys:     &sysValue,
	},
}

var sysValue int

type openOnly struct{ fs.FS }

func TestMount(t *testing.T) {
	check := func(desc string, mnt fs.FS, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("Mount(dir): %v", err)
			return
		}
		data, err := fs.ReadFile(mnt, "mount/goodbye.txt")
		if string(data) != "goodbye, world" || err != nil {
			t.Errorf(`ReadFile(%s, "goodbye.txt" = %q, %v, want %q, nil`, desc, string(data), err, "goodbye, world")
		}

		dirs, err := fs.ReadDir(mnt, ".")
		if err != nil || len(dirs) != 2 || dirs[0].Name() != "goodbye.txt" {
			var names []string
			for _, d := range dirs {
				names = append(names, d.Name())
			}
			t.Errorf(`ReadDir(%s, ".") = %v, %v, want %v, nil`, desc, names, err, []string{"mount/goodbye.txt"})
		}
	}

	// Test that Mount uses Open when the method is not present.
	mnt, err := Mount(openOnly{testFsys}, "mount")
	check("openOnly", mnt, err)

	_, err = mnt.Open("mount/nonexist")
	if err == nil {
		t.Fatal("Open(mount/nonexist): succeeded")
	}
}
