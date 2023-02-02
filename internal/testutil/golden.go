// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package testutil

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

var updateRegex = flag.String("update", "", "Update testdata of tests matching the given regex")

// Update returns true if update regex matches given test name.
func Update(name string) bool {
	if updateRegex == nil || *updateRegex == "" {
		return false
	}
	return regexp.MustCompile(*updateRegex).MatchString(name)
}

// AssertGolden compares what's got and what's in the golden file. It updates
// the golden file on-demand. It does nothing when the runtime is "windows".
func AssertGolden(t testing.TB, path string, update bool, got any) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	t.Helper()

	data := marshal(t, got)

	if update {
		err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
		if err != nil {
			t.Fatalf("create directories for golden file %q: %v", path, err)
		}

		err = ioutil.WriteFile(path, data, 0640)
		if err != nil {
			t.Fatalf("update golden file %q: %v", path, err)
		}
	}

	golden, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %q: %v", path, err)
	}

	assert.Equal(t, string(golden), string(data))
}

func marshal(t testing.TB, v any) []byte {
	t.Helper()

	switch v2 := v.(type) {
	case string:
		return []byte(v2)
	case []byte:
		return v2
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
}
