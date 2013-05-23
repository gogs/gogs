// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package utils

import (
	"testing"
)

var remotePaths = []string{
	"github.com/coocood/qbs",
	"code.google.com/p/draw2d",
	"launchpad.net/goamz",
	"bitbucket.org/gotamer/conv",
}

var importPaths = []string{
	"github.com/coocood/qbs/test",
	"code.google.com/p/draw2d/test",
	"launchpad.net/goamz/test",
	"bitbucket.org/gotamer/conv/test",
}

func TestIsValidRemotePath(t *testing.T) {
	for _, p := range remotePaths {
		if !IsValidRemotePath(p) {
			t.Errorf("Invalid remote path: %s", p)
		}
	}
}

func TestGetProjectPath(t *testing.T) {
	// Should return same path.
	for _, p := range remotePaths {
		if p != GetProjectPath(p) {
			t.Errorf("Fail to get projet path: %s", p)
		}
	}

	// Should return same path for remote paths.
	for i, p := range remotePaths {
		if remotePaths[i] != GetProjectPath(p) {
			t.Errorf("Fail to verify projet path: %s", p)
		}
	}
}
